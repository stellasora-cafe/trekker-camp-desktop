package ui

import "testing"

func TestWordBreakAllInteractionAndLayoutCache(t *testing.T) {
	var paragraph, link *node
	breakAll := false
	font := Font{Size: 20}
	width, _ := ShapeText("abc def", font, 0).Size()
	width += .1
	tt := coreNewTester(func(c *context) {
		paragraph = coreRichText(c).Width(width).FontSize(20).Selectable()
		if breakAll {
			paragraph.WordBreakAll()
		}
		paragraph.Children(func() {
			coreText(c, "abc ")
			link = coreLink(c, "defghijkl", "https://example.com/")
		})
	}, 300, 300)
	first := paragraph.tl.Lines[0].End
	breakAll = true
	tt.Frame()
	if paragraph.tl.Lines[0].End <= first {
		t.Fatal("WordBreakAll did not invalidate paragraph layout")
	}
	if paragraph.text != "abc defghijkl" || len(link.frags) < 2 {
		t.Fatalf("text or link fragments lost: %q %+v", paragraph.text, link.frags)
	}
	for _, fragment := range link.frags {
		tt.ClickAt(center(fragment))
	}
	if len(tt.OpenedURLs()) != len(link.frags) {
		t.Fatal("broken link fragments lost hit targets")
	}
	native := ShapeRichTextWith([]Span{{Text: paragraph.text}}, font, TextShape{Width: width, WordBreakAll: true})
	if native.l.Lines[0].End != paragraph.tl.Lines[0].End {
		t.Fatal("public retained shape used different word breaking")
	}
}

func TestTextLineCountReportsClampWithoutChangingSelectionText(t *testing.T) {
	var count int
	var truncated bool
	var paragraph *node
	tt := coreNewTester(func(c *context) {
		paragraph = coreText(c, "one\ntwo\nthree").MaxLines(2).Width(160).Selectable()
		paragraph.DrawOver(func(_ *Painter, _ Rect) { count, truncated = paragraph.TextLineCount() })
	}, 200, 200)
	tt.Frame()
	if count != 2 || !truncated || paragraph.text != "one\ntwo\nthree" {
		t.Fatalf("count=%d truncated=%v text=%q", count, truncated, paragraph.text)
	}
}
