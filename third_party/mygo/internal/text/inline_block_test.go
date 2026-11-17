package text

import (
	"strings"
	"testing"
)

func inlineBlockFixture(content string, width float32, anywhere bool) *Layout {
	return Shared().Layout(Params{Text: "before " + content + " after", Style: Style{Size: 20}, Width: width,
		InlineBoxes: EncodeInlineBoxes([]InlineBox{{Start: 7, End: 7 + len([]rune(content)), Style: Style{Size: 20}, Anywhere: anywhere}})})
}

func TestInlineBlockLayout(t *testing.T) {
	for _, content := range []string{"short", "one two three four five six", "第一行\n第二行"} {
		l := inlineBlockFixture(content, 160, false)
		b := &l.Boxes[0]
		if string(l.Runes) != "before "+content+" after" {
			t.Fatal("original text changed")
		}
		for _, line := range l.Lines {
			if line.End > b.Start && line.End < b.End {
				t.Fatalf("outer break inside box: %+v", line)
			}
		}
		if strings.ContainsAny(content, " \n") && len(b.Layout.Lines) < 2 {
			t.Fatalf("box contents did not wrap: %+v", b)
		}
		if b.Rect.W > 160 {
			t.Fatalf("shrink-to-fit width = %v", b.Rect.W)
		}
		for _, line := range l.Lines {
			for _, g := range line.Glyphs {
				if g.Box && abs(g.Y-(b.ContentY+b.Layout.Lines[len(b.Layout.Lines)-1].Baseline)) > .01 {
					t.Fatal("last baseline alignment lost")
				}
			}
		}
		for i := b.Start; i < b.End; i++ {
			x, y, h := l.Caret(i)
			wantX, wantY, wantH := b.Layout.Caret(i - b.Start)
			if abs(x-wantX-b.ContentX) > .01 || abs(y-wantY-b.ContentY) > .01 || h != wantH {
				t.Fatalf("caret %d does not use child geometry", i)
			}
			if got := l.IndexAt(x+.05, y+h/2); got != i {
				t.Fatalf("hit %d => %d at %v,%v", i, got, x, y)
			}
		}
		if len(l.Selection(b.Start, b.End)) != len(b.Layout.Selection(0, b.End-b.Start)) {
			t.Fatal("selection did not follow internal lines")
		}
	}
}

func TestInlineBlockIntrinsicWrap(t *testing.T) {
	word := strings.Repeat("long", 12)
	plain := inlineBlockFixture(word, 120, false)
	anywhere := inlineBlockFixture(word, 120, true)
	if plain.Boxes[0].Rect.W <= 120 || len(plain.Boxes[0].Layout.Lines) != 1 {
		t.Fatal("unbreakable min-content width not preserved")
	}
	if anywhere.Boxes[0].Rect.W > 120 || len(anywhere.Boxes[0].Layout.Lines) < 2 {
		t.Fatal("overflow-wrap:anywhere did not reduce min-content width")
	}
}

func TestInlineBlockNestedStyledText(t *testing.T) {
	p := Params{Text: "A outer inner words tail Z", Width: 140, Style: Style{Size: 20}, Spans: EncodeSpans([]Span{{End: 2}, {End: 8, Size: 30, Weight: 700}, {End: 19, Size: 12}, {End: 26}}),
		InlineBoxes: EncodeInlineBoxes([]InlineBox{{Start: 2, End: 24, Style: Style{Size: 20}}, {Start: 8, End: 19, Style: Style{Size: 12}, Insets: [4]float32{2, 3, 4, 5}}})}
	l := Shared().Layout(p)
	if len(l.Boxes) != 1 || len(l.Boxes[0].Layout.Boxes) != 1 {
		t.Fatal("nested boxes lost")
	}
	seenBig, seenSmall := false, false
	for _, line := range l.PaintLayout().Lines {
		for _, g := range line.Glyphs {
			if g.Box {
				t.Fatal("container placeholder escaped into painter")
			}
			if g.Cluster >= 2 && g.Cluster < 8 {
				seenBig = seenBig || g.Size == 30
			}
			if g.Cluster >= 8 && g.Cluster < 19 {
				seenSmall = seenSmall || g.Size == 12
			}
		}
	}
	if !seenBig || !seenSmall {
		t.Fatal("nested span font sizes lost")
	}
	if _, ok := l.InlineBoxBounds(8, 19); !ok {
		t.Fatal("nested geometry missing")
	}
	p.Width = 300
	wider := Shared().LayoutFrom(l, p)
	if wider == l || wider.Boxes[0].Rect == l.Boxes[0].Rect {
		t.Fatalf("width change reused stale box placement: %v -> %v", l.Boxes[0].Rect, wider.Boxes[0].Rect)
	}
}

func TestInlineBlockBidiCaret(t *testing.T) {
	p := Params{Text: "אב לפני one two three אחרי", Style: Style{Size: 20}, Width: 160, RTL: true,
		InlineBoxes: EncodeInlineBoxes([]InlineBox{{Start: 8, End: 21, Style: Style{Size: 20}}})}
	l := Shared().Layout(p)
	b := l.Boxes[0]
	for i := b.Start; i < b.End; i++ {
		x, y, h := l.Caret(i)
		cx, cy, ch := b.Layout.Caret(i - b.Start)
		if abs(x-cx-b.ContentX) > .01 || abs(y-cy-b.ContentY) > .01 || h != ch {
			t.Fatalf("RTL caret %d has wrong box coordinates", i)
		}
		if got := l.PositionAt(x+.01, y+h/2).Index; got != i {
			t.Fatalf("RTL hit %d => %d", i, got)
		}
	}
}

func TestInlineBlockOuterNoWrapAndTruncation(t *testing.T) {
	s := "prefix block words suffix"
	p := Params{Text: s, Style: Style{Size: 20}, Width: 60, Spans: EncodeSpans([]Span{{End: len(s), NoBreak: 1}}),
		InlineBoxes: EncodeInlineBoxes([]InlineBox{{Start: 7, End: 18, Style: Style{Size: 20}}})}
	l := Shared().Layout(p)
	if len(l.Lines) != 1 {
		t.Fatal("inline block broke ancestor atomic range")
	}
	p.Spans = ""
	p.MaxLines = 1
	l = Shared().Layout(p)
	if len(l.Boxes) != 0 {
		t.Fatal("truncated inline block retained visible bounds")
	}
}
