package ui

import (
	"math"
	"runtime"
	"testing"
)

func TestShapeRichTextWithPreWrap(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("reference uses Windows Segoe UI")
	}
	font := Font{Family: "Segoe UI", Size: 20, LineHeight: 1.35}
	shape := TextShape{Width: 50, PreWrap: true, TabSize: 2}
	l := ShapeRichTextWith([]Span{{Text: "A          B"}}, font, shape)
	if len(l.Lines()) != 2 || math.Abs(float64(l.Caret(12).X-11.46875)) > 1 {
		t.Fatalf("hanging spaces changed wrapping: %+v, caret=%+v", l.Lines(), l.Caret(12))
	}
	spaces := l.SelectionRects(TextInputRange{Start: 1, End: 11})
	if len(spaces) != 1 || spaces[0].X+spaces[0].W <= 50 {
		t.Fatalf("hanging whitespace lost selection: %+v", spaces)
	}
	l = ShapeRichTextWith([]Span{{Text: "A \t B"}}, font, shape)
	if len(l.Lines()) != 1 || math.Abs(float64(l.Caret(5).X-38.86458)) > 1 {
		t.Fatalf("tab-size not applied: %+v, caret=%+v", l.Lines(), l.Caret(5))
	}
}
