package ui

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"slices"
	"testing"
)

func inlineScopeImage(view func(*context)) *image.RGBA {
	tt := coreNewTester(func(c *context) {
		coreBox(c).Fill().Background(RGB(255, 255, 255)).Children(func() {
			coreBox(c).Absolute().Left(30).Top(30).Width(260).Height(180).Children(func() { view(c) })
		})
	}, 340, 240)
	return tt.Image()
}

func inlineScopeBitmap() *Bitmap {
	im := image.NewRGBA(image.Rect(0, 0, 30, 30))
	draw.Draw(im, im.Bounds(), &image.Uniform{C: color.RGBA{R: 255, A: 255}}, image.Point{}, draw.Src)
	return NewBitmap(im)
}

// The same group of glyphs, decorations and objects paints identically when
// it is a root paragraph or a nested inline scope, including overlapping ink.
func TestInlineScopeComposite(t *testing.T) {
	bitmap := inlineScopeBitmap()
	for _, tc := range []struct {
		name        string
		alpha, blur float32
	}{
		{"blur", 1, 3}, {"opacity", .5, 0}, {"blur_opacity", .5, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			view := func(inline bool) func(*context) {
				return func(c *context) {
					root := coreRichText(c).FontSize(24).TextColor(RGB(0, 0, 0)).Width(180)
					contents := func() {
						coreText(c, "MMMM").Background(RGB(180, 220, 255)).Underline()
						coreRichText(c, Span{Text: "\ufffc", ObjectWidth: 30, ObjectHeight: 30}).Translate(-20, 0)
						coreText(c, " more wrapped text")
					}
					if inline {
						root.Children(func() { coreRichText(c).Opacity(tc.alpha).Blur(tc.blur).Children(contents) })
					} else {
						root.Opacity(tc.alpha).Blur(tc.blur).Children(contents)
					}
					root.DrawObjects(func(p *Painter, _ int, r Rect) { p.Image(bitmap, r, Contain) })
				}
			}
			a, b := inlineScopeImage(view(false)), inlineScopeImage(view(true))
			if !bytes.Equal(a.Pix, b.Pix) {
				n := 0
				box := image.Rectangle{}
				for y := 0; y < 240; y++ {
					for x := 0; x < 340; x++ {
						if a.RGBAAt(x, y) != b.RGBAAt(x, y) {
							n++
							box = box.Union(image.Rect(x, y, x+1, y+1))
						}
					}
				}

				t.Fatalf("inline group differs from root: %d pixels %v", n, box)
			}
		})
	}
}

func TestInlineScopeNestedOpacity(t *testing.T) {
	bitmap := inlineScopeBitmap()
	view := func(nested bool) func(*context) {
		return func(c *context) {
			coreRichText(c).FontSize(30).TextColor(RGB(0, 0, 0)).Children(func() {
				contents := func() { coreText(c, "MMMM"); coreRichText(c, Span{Text: "\ufffc", ObjectWidth: 30, ObjectHeight: 30}) }
				if nested {
					coreRichText(c).Opacity(.5).Children(func() { coreRichText(c).Opacity(.5).Children(contents) })
				} else {
					coreRichText(c).Opacity(.25).Children(contents)
				}
			}).DrawObjects(func(p *Painter, _ int, r Rect) { p.Image(bitmap, r, Contain) })
		}
	}
	a, b := inlineScopeImage(view(false)), inlineScopeImage(view(true))
	for i := range a.Pix {
		d := int(a.Pix[i]) - int(b.Pix[i])
		if d < -1 || d > 1 {
			t.Fatalf("nested opacity differs at byte %d: %d/%d", i, a.Pix[i], b.Pix[i])
		}
	}
}

func TestInlineScopePreservesLayoutAndSelection(t *testing.T) {
	var paragraph, blurred, sharp *node
	blur := float32(0)
	tt := coreNewTester(func(c *context) {
		coreBox(c).Fill().Background(RGB(255, 255, 255)).Padding(20).Children(func() {
			paragraph = coreRichText(c).FontSize(24).TextColor(RGB(0, 0, 0)).Selectable().Children(func() {
				sharp = coreText(c, "sharp ")
				blurred = coreText(c, "blur these words across several lines ").Blur(blur)
				coreLink(c, "sharp link", "https://example.com/")
			})
		})
	}, 200, 260)
	before := tt.Image()
	layout := paragraph.tl
	frags := slices.Clone(blurred.frags)
	selection := layout.Selection(3, len(layout.Runes)-3)
	sharpBox := Rect{sharp.x, sharp.y, sharp.w, sharp.h}
	blur = 3
	tt.Frame()
	after := tt.Image()
	if paragraph.tl != layout || !slices.Equal(frags, blurred.frags) || !slices.Equal(selection, paragraph.tl.Selection(3, len(layout.Runes)-3)) {
		t.Fatal("painting changed paragraph layout or selection geometry")
	}
	if len(frags) < 2 {
		t.Fatal("fixture did not wrap")
	}
	// Stay well clear of the following blur's ink.
	for y := int(sharpBox.Y); y < int(sharpBox.Y+sharpBox.H); y++ {
		for x := int(sharpBox.X); x < int(sharpBox.X+sharpBox.W)-10; x++ {
			if before.RGBAAt(x, y) != after.RGBAAt(x, y) {
				t.Fatalf("neighbor text blurred at %d,%d", x, y)
			}
		}
	}
	if bytes.Equal(before.Pix, after.Pix) {
		t.Fatal("blur did not paint")
	}
	// Painting selection also leaves the original shared layout intact.
	paragraph.st.editor.anchor, paragraph.st.editor.caret = 3, len(layout.Runes)-3
	tt.Frame()
	if paragraph.tl != layout {
		t.Fatal("selection reshaped paragraph")
	}
}

func TestResolvedText(t *testing.T) {
	var f Font
	var color Color
	want := RGB(20, 80, 140)
	coreNewTester(func(c *context) {
		coreRichText(c).Font("monospace").FontSize(23).Bold().TextColor(want).Children(func() {
			f, color = coreText(c, "child").Italic().Opacity(.5).ResolvedText()
		})
	}, 200, 100)
	if f.Family != "monospace" || f.Size != 23 || f.Weight != 700 || !f.Italic || color != want {
		t.Fatalf("resolved font/color: %+v %+v", f, color)
	}
}

func TestInlineScopeInsetClip(t *testing.T) {
	bitmap := inlineScopeBitmap()
	img := inlineScopeImage(func(c *context) {
		coreRichText(c).Children(func() {
			coreRichText(c).ClipInsetPercent(0, 50, 0, 0, 0).Opacity(.5).Children(func() {
				coreRichText(c, Span{Text: "\ufffc", ObjectWidth: 30, ObjectHeight: 30})
			})
		}).DrawObjects(func(p *Painter, _ int, r Rect) { p.Image(bitmap, r, Contain) })
	})
	if c := img.RGBAAt(35, 35); c.R != 255 || c.G < 120 || c.G > 135 {
		t.Fatalf("visible half: %v", c)
	}
	if c := img.RGBAAt(50, 35); !isWhite(c) {
		t.Fatalf("hidden half: %v", c)
	}
}

func TestInlineScopeEmptySpan(t *testing.T) {
	inlineScopeImage(func(c *context) {
		coreRichText(c, Span{Text: ""}).Children(func() {
			coreText(c, "")
			coreRichText(c, Span{Text: ""}, Span{Text: "shown"}).Opacity(.5)
		})
	})
}

func TestInlineScopeDrawCallbacks(t *testing.T) {
	var order []string
	img := inlineScopeImage(func(c *context) {
		coreRichText(c).Children(func() {
			coreRichText(c, Span{Text: "\ufffc", ObjectWidth: 90, ObjectHeight: 30}).Translate(10, 0).Opacity(.5).Effect(imageEffect{}).
				Draw(func(p *Painter, r Rect) { order = append(order, "before"); p.Fill(r, RGB(255, 0, 0), 0) }).
				DrawOver(func(p *Painter, r Rect) {
					order = append(order, "after")
					p.Fill(Rect{r.X, r.Y, r.W / 2, r.H}, RGB(0, 0, 255), 0)
				})
		})
	})
	if len(order) < 2 || order[0] != "before" || order[1] != "after" {
		t.Fatalf("draw order: %v", order)
	}
	if c := img.RGBAAt(45, 35); c.B != 255 || c.R < 120 || c.R > 135 {
		t.Fatalf("draw-over not composited: %v", c)
	}
	if c := img.RGBAAt(90, 35); c.R != 255 || c.B < 120 || c.B > 135 {
		t.Fatalf("draw not composited: %v", c)
	}
	if c := img.RGBAAt(35, 35); !isWhite(c) {
		t.Fatalf("callback ignored inline transform: %v", c)
	}
}

func TestInlineScopeEffectOwnsOnlyItsSubtree(t *testing.T) {
	fx := &layerSpy{ids: map[uint64]bool{}}
	img := inlineScopeImage(func(c *context) {
		coreRichText(c).Children(func() {
			coreText(c, "hidden").FontSize(24).Effect(fx)
			coreText(c, "shown").FontSize(24).TextColor(RGB(255, 0, 0)).Translate(-40, 0)
		})
	})
	if len(fx.ids) != 1 {
		t.Fatal("inline effect was not called")
	}
	red := 0
	for y := 30; y < 70; y++ {
		for x := 30; x < 130; x++ {
			if isRed(img.RGBAAt(x, y)) {
				red++
			}
		}
	}
	if red == 0 {
		t.Fatal("inline effect captured the adjacent overlapping text")
	}
}

func TestInlineScopeDrawOverOutsideOwnContentClip(t *testing.T) {
	var box Rect
	img := inlineScopeImage(func(c *context) {
		coreRichText(c).Children(func() {
			coreText(c, "MMMM").FontSize(30).Border(6, RGBA(0, 0, 0, 0)).Clip().Opacity(.5).
				DrawOver(func(p *Painter, r Rect) { box = r; p.Stroke(r, RGB(255, 0, 0), 0, 6) })
		})
	})
	if c := img.RGBAAt(int(box.X+2), int(box.Y+2)); c.R != 255 || c.G < 120 || c.G > 135 {
		t.Fatalf("own content clip hid border: %v", c)
	}
}
