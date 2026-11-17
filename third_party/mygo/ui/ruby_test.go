package ui

import (
	"bytes"
	"image"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func rubyFixtureFont(t *testing.T) string {
	t.Helper()
	for _, face := range []string{"Medium", "Bold"} {
		path := filepath.Join("..", "..", "..", "..", "misskey", "packages", "frontend", "assets", "camp", "fonts", "misans-v1", "MiSans-"+face+".ttf")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Skip("MiSans local parity assets are unavailable")
		}
		if err = RegisterFont(data, "Ruby Fixture"); err != nil {
			t.Fatal(err)
		}
	}
	return "Ruby Fixture"
}

func TestRubyChromiumGeometry(t *testing.T) {
	family := rubyFixtureFont(t)
	for _, tc := range []struct {
		name, base, annotation, prefix, suffix string
		size, width, height, baseline          float32
	}{
		{"short", "漢字", "かんじ", "前 ", " 后", 24, 48, 41.72917, 34.66667},
		{"wide", "字", "lengthyannotation", "前 ", " 后", 24, 89.875, 41.72917, 34.66667},
		{"latin", "AB", "annotation", "前 ", " 后", 24, 48.1875, 41.72917, 34.66667},
		{"large", "AB", "annotation", "前 ", " 后", 48, 64.90625, 70.125, 56},
		{"alone", "字", "lengthyannotation", "", "", 24, 101.875, 41.72917, 34.66667},
		{"before", "字", "lengthyannotation", "前 ", "", 24, 95.875, 41.72917, 34.66667},
		{"after", "字", "lengthyannotation", "", " 后", 24, 95.875, 41.72917, 34.66667},
		{"multi", "漢字", "かんじ", "段落第一行\n第二行", "结束\n第三行", 24, 48, 102.125, 95.0625},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var paragraph, ruby *node
			tt := coreNewTester(func(c *context) {
				paragraph = coreRichText(c).Font(family).FontSize(24).LineHeight(1.35).Width(600).Children(func() {
					if tc.prefix != "" {
						coreText(c, tc.prefix)
					}
					ruby = coreRichText(c).Ruby(tc.annotation).Children(func() { coreText(c, tc.base).FontSize(tc.size) })
					if tc.suffix != "" {
						coreText(c, tc.suffix)
					}
				})
			}, 650, 160)
			tt.SetScale(1.5)
			if len(paragraph.tl.Lines) != 1 && tc.name != "multi" {
				t.Fatalf("unexpected lines: %+v", paragraph.tl.Lines)
			}
			line := paragraph.tl.Lines[len(paragraph.tl.Lines)-1]
			reserved := ruby.ruby.width - ruby.ruby.overhang
			_, after := ruby.rubyNeighbors(paragraph.textParams(0).Style)
			reserved -= min(min(ruby.ruby.inset, 6), after/2)
			if math.Abs(float64(reserved-tc.width)) > 1 || math.Abs(float64(paragraph.tl.Height-tc.height)) > 1 || math.Abs(float64(line.Baseline-tc.baseline)) > 1 {
				t.Fatalf("geometry differs from Chromium: width=%g height=%g baseline=%g", reserved, paragraph.tl.Height, line.Baseline)
			}
		})
	}
}

func TestRubyKeepsOriginalTextAndStyledLink(t *testing.T) {
	var paragraph, ruby, link *node
	tt := coreNewTester(func(c *context) {
		paragraph = coreRichText(c).FontSize(24).Width(90).Selectable().Children(func() {
			coreText(c, "prefix ")
			ruby = coreRichText(c).Ruby("long annotation").TextColor(RGB(0, 180, 0)).Children(func() {
				coreText(c, "中").Bold().TextColor(RGB(255, 0, 0))
				link = coreLink(c, "linked words", "https://example.com/")
			})
			coreText(c, " suffix")
		})
	}, 400, 240)
	if paragraph.text != "prefix 中linked words suffix" || strings.ContainsRune(paragraph.text, '\ufffc') {
		t.Fatal("ruby changed base text", paragraph.text)
	}
	if len(ruby.frags) != 1 {
		t.Fatal("ruby broke across lines", ruby.frags)
	}
	tt.ClickAt(center(link.frags[0]))
	if len(tt.OpenedURLs()) != 1 {
		t.Fatal("ruby link lost interaction")
	}
}

func TestRubyChromiumDisplayScales(t *testing.T) {
	family := rubyFixtureFont(t)
	for _, tc := range []struct{ scale, height, baseline, annotationTop float32 }{
		{1, 42.39063, 35, -2}, {1.25, 42, 34.4, -1.6}, {1.5, 41.72917, 34.66667, -1.33333}, {2, 41.39844, 34, -1.5},
	} {
		var paragraph, ruby *node
		tt := coreNewTester(func(c *context) {
			paragraph = coreRichText(c).Font(family).FontSize(24).LineHeight(1.35).Children(func() { ruby = coreRichText(c).Ruby("かんじ").Children(func() { coreText(c, "漢字") }) })
		}, 240, 120)
		tt.SetScale(tc.scale)
		line := paragraph.tl.Lines[0]
		top := line.Baseline - ruby.ruby.offset - ruby.ruby.layout.Lines[0].Baseline
		if math.Abs(float64(line.Height-tc.height)) > 1 || math.Abs(float64(line.Baseline-tc.baseline)) > 1 || math.Abs(float64(top-tc.annotationTop)) > 1 {
			t.Fatalf("scale %g: height=%g baseline=%g top=%g", tc.scale, line.Height, line.Baseline, top)
		}
	}
}

func TestRubyInheritsInlinePaintScopes(t *testing.T) {
	for _, tc := range []struct {
		name        string
		alpha, blur float32
	}{{"opacity", .5, 0}, {"blur", 1, 3}, {"both", .5, 3}} {
		t.Run(tc.name, func(t *testing.T) {
			view := func(nested bool) func(*context) {
				return func(c *context) {
					paragraph := coreRichText(c).FontSize(24).Width(180).TextColor(RGB(0, 0, 0))
					body := func() { coreRichText(c).Ruby("annotation").Children(func() { coreText(c, "AB") }) }
					if nested {
						paragraph.Children(func() { coreRichText(c).Background(RGB(255, 255, 255)).Opacity(tc.alpha).Blur(tc.blur).Children(body) })
					} else {
						paragraph.Background(RGB(255, 255, 255)).Opacity(tc.alpha).Blur(tc.blur).Children(body)
					}
				}
			}
			a := inlineScopeImage(view(false))
			b := inlineScopeImage(view(true))
			if !bytes.Equal(a.Pix, b.Pix) {
				n, delta := 0, 0
				box := image.Rectangle{}
				var point image.Point
				for y := 0; y < a.Bounds().Dy(); y++ {
					for x := 0; x < a.Bounds().Dx(); x++ {
						if a.RGBAAt(x, y) != b.RGBAAt(x, y) {
							n++
							box = box.Union(image.Rect(x, y, x+1, y+1))
							for channel := 0; channel < 4; channel++ {
								diff := int(math.Abs(float64(int(a.Pix[a.PixOffset(x, y)+channel]) - int(b.Pix[b.PixOffset(x, y)+channel]))))
								if diff > delta {
									delta = diff
									point = image.Pt(x, y)
								}
							}
						}
					}
				}
				// Blur samples two differently sized layer surfaces; their byte
				// quantization may differ by two levels at the filter boundary.
				if tc.blur == 0 || delta > 2 {
					t.Fatalf("ruby differs between inline and paragraph paint scope: count=%d delta=%d box=%v peak%v=%v/%v", n, delta, box, point, a.RGBAAt(point.X, point.Y), b.RGBAAt(point.X, point.Y))
				}
			}
		})
	}
}

func TestRubyInlineBlockBaseUsesFullFontBox(t *testing.T) {
	family := rubyFixtureFont(t)
	var paragraph, ruby *node
	tt := coreNewTester(func(c *context) {
		paragraph = coreRichText(c).Font(family).FontSize(24).LineHeight(1.35).Width(600).Children(func() {
			coreText(c, "前 ")
			ruby = coreRichText(c).Ruby("annotation").Children(func() {
				coreRichText(c).Bold().Children(func() { coreText(c, "AB").InlineBlock() })
			})
			coreText(c, " 后")
		})
	}, 650, 160)
	tt.SetScale(1.5)
	if math.Abs(float64(paragraph.tl.Height-45.0625)) > 1 || math.Abs(float64(paragraph.tl.Lines[0].Baseline-38)) > 1 {
		t.Fatalf("full inline box differs from Chromium: height=%g baseline=%g offset=%g boxes=%+v", paragraph.tl.Height, paragraph.tl.Lines[0].Baseline, ruby.ruby.offset, paragraph.tl.Boxes)
	}
}

func TestRubyNestedFormattingAndContainers(t *testing.T) {
	family := rubyFixtureFont(t)
	for _, name := range []string{"outerBlock", "plainWrapper", "nested", "nestedBlocks"} {
		t.Run(name, func(t *testing.T) {
			var paragraph, ruby *node
			tt := coreNewTester(func(c *context) {
				paragraph = coreRichText(c).Font(family).FontSize(24).LineHeight(1.35).Width(600).Children(func() {
					coreText(c, "前 ")
					body := func() { ruby = coreText(c, "字").Ruby("lengthyannotation") }
					switch name {
					case "outerBlock":
						coreRichText(c).InlineBlock().Children(body)
					case "plainWrapper":
						coreRichText(c).Bold().Children(body)
					case "nested":
						ruby = coreRichText(c).Ruby("outer annotation").Children(func() { coreText(c, "漢字").Ruby("かんじ") })
					case "nestedBlocks":
						coreRichText(c).InlineBlock().Children(func() {
							coreRichText(c).Bold().InlineBlock().Children(func() { ruby = coreText(c, "漢字").Ruby("かんじ") })
						})
					}
					coreText(c, " 后")
				})
			}, 650, 160)
			tt.SetScale(1.5)
			wantHeight, wantBaseline, wantWidth := float32(41.72917), float32(34.66667), float32(101.875)
			switch name {
			case "plainWrapper":
				wantWidth = 100.85417
			case "nested":
				wantHeight, wantBaseline, wantWidth = 54.39583, 47.33333, 81.5
			case "nestedBlocks":
				wantWidth = 48
			}
			line := paragraph.tl.Lines[0]
			_, after := ruby.rubyNeighbors(paragraph.textParams(0).Style)
			width := ruby.ruby.width - ruby.ruby.overhang - min(min(ruby.ruby.inset, 6), after/2)
			if math.Abs(float64(paragraph.tl.Height-wantHeight)) > 1 || math.Abs(float64(line.Baseline-wantBaseline)) > 1 || math.Abs(float64(width-wantWidth)) > 1 {
				t.Fatalf("Chromium mismatch: height=%g baseline=%g width=%g", paragraph.tl.Height, line.Baseline, width)
			}
			if name == "nested" {
				inner := ruby.first.ruby
				if math.Abs(float64(inner.layout.Width-70.75)) > 1 || math.Abs(float64(inner.anchor-ruby.ruby.anchor)) > 1 {
					t.Fatalf("nested annotation justification: width=%g anchor=%g outerAnchor=%g", inner.layout.Width, inner.anchor, ruby.ruby.anchor)
				}
			}
			img := tt.Image()
			dark := 0
			for y := 0; y < int(line.Baseline-24); y++ {
				for x := 0; x < img.Bounds().Dx(); x++ {
					if img.RGBAAt(x, y).R < 150 {
						dark++
					}
				}
			}
			if dark == 0 {
				t.Fatal("annotation missing above base")
			}
		})
	}
}

func TestRubyAdjacentAnnotationsDoNotOverhangEachOther(t *testing.T) {
	family := rubyFixtureFont(t)
	var paragraph *node
	tt := coreNewTester(func(c *context) {
		paragraph = coreRichText(c).Font(family).FontSize(24).LineHeight(1.35).Children(func() {
			for range 2 {
				coreText(c, "字").Ruby("lengthyannotation")
			}
		})
	}, 650, 160)
	tt.SetScale(1.5)
	if math.Abs(float64(paragraph.tl.Width-203.75)) > 1 {
		t.Fatalf("adjacent annotation width=%g, Chromium=203.75", paragraph.tl.Width)
	}
}

func TestRubyDistributionKeepsLatinAndCombiningClusters(t *testing.T) {
	for _, tc := range []struct {
		text   string
		groups []int
	}{
		{"かんじ", []int{0, 1, 2}}, {"annotation", []int{0, 0, 0, 0, 0, 0, 0, 0, 0, 0}}, {"かanじ", []int{0, 1, 1, 2}}, {"e\u0301", []int{0, 0}},
	} {
		spans, inset := rubyExpand([]Span{{Text: tc.text}}, 24)
		var text string
		var total float32
		for _, sp := range spans {
			text += sp.Text
			total += sp.leading + sp.trailing
		}
		if text != tc.text || math.Abs(float64(total-24)) > .01 || inset <= 0 {
			t.Fatal("distribution changed text or spacing", tc.text, spans)
		}
		groups, _ := rubyGroups([]rune(tc.text))
		for i, g := range groups {
			if g != tc.groups[i] {
				t.Fatal("split typographic group", tc.text, groups)
			}
		}
	}
}

func TestRubyEmptyBaseAndDynamicAnnotation(t *testing.T) {
	family := rubyFixtureFont(t)
	annotation := "annotation"
	var paragraph, ruby *node
	tt := coreNewTester(func(c *context) {
		paragraph = coreRichText(c).Font(family).FontSize(24).LineHeight(1.35).Children(func() {
			coreText(c, "前 ")
			ruby = coreRichText(c).Ruby(annotation).Children(func() {
				coreRichText(c, Span{Text: "\ufffc", ObjectWidth: .001, ObjectHeight: 25.344, ObjectDescent: 7.056})
			})
			coreText(c, " 后")
		})
	}, 650, 160)
	tt.SetScale(1.5)
	if math.Abs(float64(paragraph.tl.Height-45.0625)) > 1 || math.Abs(float64(paragraph.tl.Lines[0].Baseline-38)) > 1 || math.Abs(float64(ruby.ruby.width-60.1875)) > 1 || ruby.ruby.overhang != 0 {
		t.Fatalf("empty base differs from Chromium: height%g baseline%g width%g overhang%g", paragraph.tl.Height, paragraph.tl.Lines[0].Baseline, ruby.ruby.width, ruby.ruby.overhang)
	}
	before := tt.Image()
	annotation = "lengthyannotation"
	tt.Frame()
	if ruby.ruby.width < 100 || bytes.Equal(before.Pix, tt.Image().Pix) {
		t.Fatal("changed annotation retained stale layout")
	}
}

func TestRubyGradientAndNestedColor(t *testing.T) {
	img := inlineScopeImage(func(c *context) {
		coreRichText(c).FontSize(28).TextGradient(LinearGradient{From: RGB(0, 0, 255), To: RGB(0, 0, 255)}).Children(func() {
			coreRichText(c).Ruby("annotation").Translate(12, 5).Opacity(.5).Children(func() { coreText(c, "AB").TextColor(RGB(255, 0, 0)) })
		})
	})
	red, blue := image.Rectangle{}, image.Rectangle{}
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			c := img.RGBAAt(x, y)
			if c.G >= 145 {
				continue
			}
			if c.G < 125 {
				t.Fatalf("ruby opacity applied incorrectly: %v", c)
			}
			box := image.Rect(x, y, x+1, y+1)
			if c.R > 240 && c.B < 145 {
				red = red.Union(box)
			}
			if c.B > 240 && c.R < 145 {
				blue = blue.Union(box)
			}
		}
	}
	if red.Empty() || blue.Empty() || blue.Max.Y > red.Min.Y || blue.Min.X < 40 {
		t.Fatalf("ruby annotation lost gradient, transform or base color: red=%v blue=%v", red, blue)
	}
}
