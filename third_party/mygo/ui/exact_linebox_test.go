package ui

import (
	"math"
	"testing"
)

func TestExactInlineLineBoxPaintAndCaret(t *testing.T) {
	spans := []Span{{Text: "Hello 😀\n", Weight: 700, LineAscent: 13.333333, LineDescent: 4.125, ExactLineBox: true}, {Text: "second line", LineAscent: 13.333333, LineDescent: 4.125, ExactLineBox: true}}
	font := Font{Family: "MiSans, sans-serif", Size: 12.936, LineHeight: 1.35}
	layout := ShapeRichText(spans, font, 200)
	_, height := layout.Size()
	if math.Abs(float64(height-34.916667)) > .0001 {
		t.Fatal("explicit CSS line box height", height)
	}
	if dy := layout.Caret(9).Y - layout.Caret(0).Y; math.Abs(float64(dy-17.458333)) > .0001 {
		t.Fatal("caret detached from second line", dy)
	}
	tt := coreNewTester(func(c *context) {
		coreRichText(c).Font(font.Family).FontSize(font.Size).LineHeight(1.35).Width(200).Label("paragraph").Children(func() { coreRichText(c, spans...) })
	}, 240, 100)
	r, ok := tt.Find("paragraph")
	if !ok || math.Abs(float64(r.H-height)) > .0001 {
		t.Fatal("inline public spans lost exact line box", r)
	}
	img := tt.Image()
	ink := 0
	for y := 0; y < int(height); y++ {
		for x := 0; x < 200; x++ {
			if img.RGBAAt(x, y) != img.RGBAAt(239, 99) {
				ink++
			}
		}
	}
	if ink == 0 {
		t.Fatal("exact line box omitted glyph paint")
	}
}
