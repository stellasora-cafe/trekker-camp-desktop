package ui

import (
	"testing"
)

func TestInlineLineBoxKeepsGlyphsAndInteraction(t *testing.T) {
	var paragraph, link *node
	clicked := 0
	explicit := false
	tt := coreNewTester(func(c *context) {
		paragraph = coreRichText(c).FontSize(20).Children(func() {
			coreText(c, "A")
			span := Span{Text: "中link"}
			if explicit {
				span.LineAscent, span.LineDescent = 50, 14
			}
			link = coreRichText(c, span)
			wrapElement(link).OnClick(func() { clicked++ })
			coreText(c, "Z")
		})
	}, 300, 160)
	baseline := paragraph.tl.Lines[0].Baseline
	explicit = true
	tt.Frame()
	line := paragraph.tl.Lines[0]
	if line.Baseline != 50 || line.Height != 64 {
		t.Fatal("inline line box did not align on the paragraph baseline", line.Baseline, line.Height)
	}
	if paragraph.text != "A中linkZ" {
		t.Fatal("line metrics introduced layout characters", paragraph.text)
	}
	for _, glyph := range line.Glyphs {
		if glyph.Object || glyph.Y != 50 {
			t.Fatal("line metrics replaced or shifted a glyph", glyph)
		}
	}
	tt.ClickAt(center(link.frags[0]))
	if clicked != 1 {
		t.Fatal("line metrics detached the hit target")
	}
	explicit = false
	tt.Frame()
	if paragraph.tl.Lines[0].Baseline != baseline {
		t.Fatal("unset line metrics retained a cached line box")
	}
}

func TestInlineLineBoxComposesWithBaselineShiftAndContainer(t *testing.T) {
	var paragraph *node
	tt := coreNewTester(func(c *context) {
		paragraph = coreRichText(c).FontSize(20).Children(func() {
			coreRichText(c).InlineBlock().Children(func() { coreRichText(c, Span{Text: "raised", LineAscent: 50, LineDescent: 14, BaselineShift: 5}) })
		})
	}, 300, 160)
	_ = tt
	if len(paragraph.tl.Boxes) != 1 {
		t.Fatal("container missing")
	}
	inner := paragraph.tl.Boxes[0].Layout.Lines[0]
	if inner.Baseline != 55 || inner.Height != 64 || inner.Glyphs[0].Y != 50 {
		t.Fatal("shifted line box lost baseline separation", inner.Baseline, inner.Height, inner.Glyphs[0].Y)
	}
}
