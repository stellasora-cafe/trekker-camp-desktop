package text

import (
	"slices"
	"testing"
)

func TestBreakAllFillsLineAndPreservesText(t *testing.T) {
	s := Shared()
	style := Style{Size: 20}
	content := "abc defghijkl 中文 e\u0301 👩\u200d💻 end"
	width := s.Layout(Params{Text: "abc def", Style: style}).Width + .1
	p := Params{Text: content, Style: style, Width: width, BreakAll: true}
	l := s.Layout(p)
	if l.Lines[0].End != 7 {
		t.Fatalf("first line did not fill across the word boundary: %q", string(l.Runes[:l.Lines[0].End]))
	}
	if string(l.Runes) != content {
		t.Fatal("original text changed")
	}
	var boundaries Boundaries
	boundaries.Reset(l.Runes)
	for i, line := range l.Lines {
		if _, found := slices.BinarySearch(boundaries.graphemes, line.End); !found {
			t.Fatalf("split a grapheme at %d", line.End)
		}
		if i > 0 && line.Start != l.Lines[i-1].End {
			t.Fatal("line rune offsets changed")
		}
		for _, g := range line.Glyphs {
			if g.Cluster < line.Start || g.Cluster+g.Runes > line.End {
				t.Fatalf("glyph escaped original line range: %+v", g)
			}
		}
	}
	p.BreakAll = false
	regular := s.Layout(p)
	if regular.Lines[0].End >= l.Lines[0].End {
		t.Fatal("word breaking did not participate in shape cache key")
	}
}

func TestBreakAllAtomicRangesPunctuationAndNoWrap(t *testing.T) {
	runes := []rune("a（bc）d\u00a0e")
	opportunities := breakAllOpportunities(runes, nil)
	for _, forbidden := range []int{2, 4, 6, 7} {
		if slices.Contains(opportunities, forbidden) {
			t.Fatalf("break allowed beside protected punctuation/space at %d: %v", forbidden, opportunities)
		}
	}
	s := Shared()
	p := Params{Text: "ab CDEFG hij", Style: Style{Size: 20}, Width: 10, BreakAll: true,
		Spans: EncodeSpans([]Span{{End: 3}, {End: 5, NoBreak: 1, Weight: 700}, {End: 8, NoBreak: 1}, {End: 12}})}
	l := s.Layout(p)
	found := false
	for _, line := range l.Lines {
		if line.Start == 3 && (line.End == 8 || line.End == 9) {
			found = true
		}
		if line.End > 3 && line.End < 8 {
			t.Fatalf("atomic styled range split at %d", line.End)
		}
	}
	if !found {
		t.Fatal("oversized atomic range did not overflow intact")
	}
	p.NoWrap = true
	if got := len(s.Layout(p).Lines); got != 1 {
		t.Fatalf("NoWrap produced %d lines", got)
	}
	p.NoWrap, p.MaxLines = false, 2
	if got := s.Layout(p); len(got.Lines) != 2 || got.Lines[1].End >= len(got.Runes) {
		t.Fatal("max-lines did not truncate")
	}
}
