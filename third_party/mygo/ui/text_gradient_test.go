package ui

import (
	"testing"
)

func TestTextGradientPreservesMixedContent(t *testing.T) {
	bitmap := inlineScopeBitmap()
	var textBox, objectBox Rect
	img := inlineScopeImage(func(c *context) {
		coreRichText(c).FontSize(30).Children(func() {
			coreRichText(c).TextGradient(LinearGradient{From: RGB(255, 0, 0), To: RGB(0, 0, 255), Angle: 90}).Children(func() {
				text := coreText(c, "MMMM").Background(RGB(20, 190, 40)).TextColor(RGBA(0, 0, 0, 0))
				text.DrawOver(func(p *Painter, r Rect) { textBox = r; p.Fill(Rect{r.X, r.Y, 4, 4}, RGB(255, 220, 0), 0) })
				coreRichText(c, Span{Text: "\ufffc", ObjectWidth: 30, ObjectHeight: 30})
			})
		}).DrawObjects(func(p *Painter, _ int, r Rect) { objectBox = r; p.Image(bitmap, r, Contain) })
	})
	if c := img.RGBAAt(int(textBox.X+1), int(textBox.Y+1)); c.R != 255 || c.G != 220 || c.B != 0 {
		t.Fatalf("callback tinted: %v", c)
	}
	if c := img.RGBAAt(int(textBox.X+10), int(textBox.Y+2)); c.R != 20 || c.G != 190 || c.B != 40 {
		t.Fatalf("background tinted: %v", c)
	}
	if c := img.RGBAAt(int(objectBox.X+10), int(objectBox.Y+10)); !isRed(c) {
		t.Fatalf("image tinted: %v", c)
	}
	red, blue := 0, 0
	for y := int(textBox.Y + 5); y < int(textBox.Y+textBox.H); y++ {
		for x := int(textBox.X); x < int(textBox.X+textBox.W); x++ {
			c := img.RGBAAt(x, y)
			if c.R > 180 && c.G < 60 {
				red++
			}
			if c.B > 120 && c.G < 60 {
				blue++
			}
		}
	}
	if red == 0 || blue == 0 {
		t.Fatalf("gradient glyphs missing: red=%d blue=%d", red, blue)
	}
}

func TestTextGradientNestedOpacity(t *testing.T) {
	var box Rect
	img := inlineScopeImage(func(c *context) {
		coreRichText(c).TextGradient(LinearGradient{From: RGB(255, 0, 0), To: RGB(255, 0, 0), Angle: 90}).Children(func() {
			coreText(c, "MMMM").FontSize(30).Opacity(.5).DrawOver(func(_ *Painter, r Rect) { box = r })
		})
	})
	colored := 0
	for y := int(box.Y); y < int(box.Y+box.H); y++ {
		for x := int(box.X); x < int(box.X+box.W); x++ {
			c := img.RGBAAt(x, y)
			if c.G < 145 {
				if c.R != 255 || c.G < 125 || c.B != c.G {
					t.Fatalf("gradient alpha multiplied incorrectly: %v", c)
				}
				colored++
			}
		}
	}
	if colored == 0 {
		t.Fatal("no gradient glyphs")
	}
}

func TestTextGradientExplicitColorOverridesInheritance(t *testing.T) {
	var boxes [4]Rect
	gradient := LinearGradient{From: RGB(255, 0, 0), To: RGB(255, 0, 0), Angle: 90}
	img := inlineScopeImage(func(c *context) {
		coreRichText(c).FontSize(24).TextColor(RGBA(0, 0, 0, 0)).TextGradient(gradient).Children(func() {
			coreText(c, "MM").DrawOver(func(_ *Painter, r Rect) { boxes[0] = r })
			coreText(c, "MM").TextColor(RGB(0, 180, 0)).DrawOver(func(_ *Painter, r Rect) { boxes[1] = r })
			coreRichText(c, Span{Text: "MM", Color: RGB(0, 180, 0)}).DrawOver(func(_ *Painter, r Rect) { boxes[2] = r })
			coreText(c, "MM").TextColor(RGB(0, 180, 0)).TextGradient(gradient).DrawOver(func(_ *Painter, r Rect) { boxes[3] = r })
		})
	})
	for i, box := range boxes {
		red, green := 0, 0
		for y := int(box.Y); y < int(box.Y+box.H); y++ {
			for x := int(box.X); x < int(box.X+box.W); x++ {
				c := img.RGBAAt(x, y)
				if c.R > 200 && c.G < 80 {
					red++
				}
				if c.G > 120 && c.R < 80 {
					green++
				}
			}
		}
		if i == 1 || i == 2 {
			if green == 0 || red != 0 {
				t.Fatalf("explicit color %d: red=%d green=%d", i, red, green)
			}
		} else {
			if red == 0 || green != 0 {
				t.Fatalf("gradient %d: red=%d green=%d", i, red, green)
			}
		}
	}
}
