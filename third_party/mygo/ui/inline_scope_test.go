package ui_test

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"testing"

	"github.com/egoist/mygo/ui"
)

func inlineProbe(t *testing.T, name string, view func(*ui.Context)) *image.RGBA {
	t.Helper()
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Fill().Background(ui.RGB(255, 255, 255)).Children(func() { ui.Box(c).Absolute().Left(30).Top(30).Width(220).Height(100).Children(func() { view(c) }) })
	}, 300, 160)
	tt.Frame()
	im := tt.Image()
	return im
}

func pixelDifference(a, b *image.RGBA) int {
	count := 0
	for y := 0; y < a.Bounds().Dy(); y++ {
		for x := 0; x < a.Bounds().Dx(); x++ {
			if a.RGBAAt(x, y) != b.RGBAAt(x, y) {
				count++
			}
		}
	}
	return count
}

func TestSDKInlineLayerContracts(t *testing.T) {
	text := func(rootBlur, inlineBlur float32) func(*ui.Context) {
		return func(c *ui.Context) {
			ui.RichText(c).FontSize(32).TextColor(ui.RGB(0, 0, 0)).Blur(rootBlur).Children(func() { ui.Text(c, "MMMM ABCD").Blur(inlineBlur) })
		}
	}
	plain := inlineProbe(t, "plain", text(0, 0))
	root := inlineProbe(t, "root-blur", text(4, 0))
	inline := inlineProbe(t, "inline-blur", text(0, 4))
	t.Run("root_blur_control", func(t *testing.T) {
		diff := pixelDifference(plain, root)
		t.Logf("root blur changes %d pixels", diff)
		if diff == 0 {
			t.Fatal("root blur control did not render")
		}
	})
	t.Run("inline_blur", func(t *testing.T) {
		diff := pixelDifference(plain, inline)
		t.Logf("inline blur changes %d pixels", diff)
		if diff == 0 {
			t.Fatal("inline Blur(4) left glyph pixels unchanged")
		}
	})
	bitmapImage := image.NewRGBA(image.Rect(0, 0, 40, 40))
	draw.Draw(bitmapImage, bitmapImage.Bounds(), &image.Uniform{C: color.RGBA{R: 255, A: 255}}, image.Point{}, draw.Src)
	bitmap := ui.NewBitmap(bitmapImage)
	object := func(rootAlpha, inlineAlpha float32, clip bool) func(*ui.Context) {
		return func(c *ui.Context) {
			ui.RichText(c).Opacity(rootAlpha).Children(func() {
				container := ui.RichText(c).Opacity(inlineAlpha)
				if clip {
					container.Clip()
				}
				container.Children(func() {
					e := ui.RichText(c, ui.Span{Text: "\ufffc", ObjectWidth: 40, ObjectHeight: 40})
					if clip {
						e.Translate(30, 0)
					}
				})
			}).DrawObjects(func(p *ui.Painter, _ int, r ui.Rect) { p.Image(bitmap, r, ui.Contain) })
		}
	}
	full := inlineProbe(t, "object-full", object(1, 1, false))
	rootHalf := inlineProbe(t, "object-root-half", object(.5, 1, false))
	inlineHalf := inlineProbe(t, "object-inline-half", object(1, .5, false))
	t.Run("root_object_opacity_control", func(t *testing.T) {
		if bytes.Equal(full.Pix, rootHalf.Pix) {
			t.Fatal("root opacity control did not render")
		}
		t.Logf("root object .5 opacity changes %d pixels", pixelDifference(full, rootHalf))
	})
	t.Run("inline_object_opacity", func(t *testing.T) {
		t.Logf("inline object .5 opacity changes %d pixels", pixelDifference(full, inlineHalf))
		if bytes.Equal(full.Pix, inlineHalf.Pix) {
			t.Fatal("inline Opacity(.5) left bitmap pixels unchanged")
		}
	})
	clipped := inlineProbe(t, "object-nested-clip", object(1, 1, true))
	rootClip := inlineProbe(t, "object-root-clip", func(c *ui.Context) {
		ui.Box(c).Size(40, 40).Clip().Children(func() {
			ui.RichText(c, ui.Span{Text: "\ufffc", ObjectWidth: 40, ObjectHeight: 40}).Translate(30, 0).DrawObjects(func(p *ui.Painter, _ int, r ui.Rect) { p.Image(bitmap, r, ui.Contain) })
		})
	})
	t.Run("root_clip_control", func(t *testing.T) {
		for y := 30; y < 70; y++ {
			for x := 70; x < 100; x++ {
				p := rootClip.RGBAAt(x, y)
				if p.R > 200 && p.G < 20 && p.B < 20 {
					t.Fatal("root clip control leaked image pixels")
				}
			}
		}
	})
	t.Run("nested_inline_clip", func(t *testing.T) {
		overflow := 0
		for y := 30; y < 70; y++ {
			for x := 70; x < 100; x++ {
				p := clipped.RGBAAt(x, y)
				if p.R > 200 && p.G < 20 && p.B < 20 {
					overflow++
				}
			}
		}
		t.Logf("red pixels outside inline clip: %d", overflow)
		if overflow > 0 {
			t.Fatalf("nested inline clip leaked %d image pixels", overflow)
		}
	})
}
