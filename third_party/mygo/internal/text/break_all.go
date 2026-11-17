package text

import (
	"slices"
	"unicode"

	"github.com/go-text/typesetting/segmenter"
)

// breakAllOpportunities gives letters and numbers ideographic line-breaking
// behavior while retaining punctuation, nonbreaking spaces and the original
// grapheme boundaries. The probe is used only for segmentation: the shaper,
// selection and glyph clusters always receive the original rune stream.
func breakAllOpportunities(runes []rune, spans []Span) []int {
	probe := slices.Clone(runes)
	for i, r := range probe {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			probe[i] = '字'
		}
	}
	var boundaries Boundaries
	boundaries.Reset(runes)
	var seg segmenter.Segmenter
	seg.Init(probe)
	var ends []int
	for it := seg.LineIterator(); it.Next(); {
		line := it.Line()
		end := line.Offset + len(line.Text)
		_, grapheme := slices.BinarySearch(boundaries.graphemes, end)
		_, _, atomic := inlineRangeAt(spans, end)
		if grapheme && !atomic {
			ends = append(ends, end)
		}
	}
	return ends
}

// breakAllLines selects the longest allowed prefix that fits each line.
// Every candidate is shaped in its own context so ligatures, bidi runs and
// joining scripts retain the same glyphs as the final line.
func (s *System) breakAllLines(runes []rune, style Style, spans []Span, width, indent float32, rtl bool, hy *hyphens) []shapedLine {
	ends := breakAllOpportunities(runes, spans)
	e := s.engine()
	var out []shapedLine
	for pos, first := 0, 0; pos < len(runes) && first < len(ends); {
		limit := width
		if pos == 0 {
			limit = max(width-indent, 1)
		}
		measure := func(end int) ([]shapedLine, float32) {
			local := spansIn(spans, pos, end)
			lines := e.shape(runes[pos:end], style, local, 0, rtl, false)
			w := float32(0)
			for _, line := range lines {
				advance, padding := paddedWidth(line, runes[pos:end], local)
				w = max(w, advance+padding)
			}
			if end < len(runes) && runes[end-1] == softHyphen && hy != nil {
				w += hy.width(end - 1)
			}
			return lines, w
		}
		best := first
		kept, _ := measure(ends[first])
		for lo, hi := first+1, len(ends)-1; lo <= hi; {
			mid := lo + (hi-lo)/2
			lines, w := measure(ends[mid])
			if w <= limit+1e-3 {
				best, kept, lo = mid, lines, mid+1
			} else {
				hi = mid - 1
			}
		}
		offsetLines(kept, pos)
		out = append(out, kept...)
		pos, first = ends[best], best+1
	}
	return out
}
