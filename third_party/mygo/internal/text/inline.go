package text

import "slices"

// breakInlineRanges follows the engine's line breaks, moving a break that
// falls inside an atomic inline range to its start. A range at the start of
// a line may overflow it. Each retained slice is shaped with its original
// styles, bidi ordering and object metrics; no replacement glyphs enter the
// text or its selection offsets.
func (s *System) breakInlineRanges(runes []rune, style Style, spans []Span, width, indent float32, rtl, wholeWords bool) []shapedLine {
	var out []shapedLine
	for pos := 0; pos < len(runes); {
		local := spansIn(spans, pos, len(runes))
		firstIndent := float32(0)
		if pos == 0 {
			firstIndent = indent
		}
		rest := runes[pos:]
		var hy *hyphens
		if slices.Contains(rest, softHyphen) {
			hy = &hyphens{e: s.engine(), style: style, spans: local}
		}
		lines := s.breakLinesRegular(rest, style, local, width, firstIndent, rtl, wholeWords, hy)
		if len(lines) == 0 {
			break
		}
		offsetLines(lines, pos)
		for _, line := range lines {
			a, b, inside := inlineRangeAt(spans, line.end)
			if inside {
				end := a
				if a <= line.start {
					end = b
				}
				kept := s.engine().shape(runes[line.start:end], style, spansIn(spans, line.start, end), 0, rtl, wholeWords)
				offsetLines(kept, line.start)
				out = append(out, kept...)
				break
			}
			out = append(out, line)
		}
		end := out[len(out)-1].end
		if end <= pos {
			break
		}
		pos = end
	}
	return out
}

func inlineRangeAt(spans []Span, at int) (start, end int, inside bool) {
	from := 0
	for i := 0; i < len(spans); {
		sp := spans[i]
		start, end = from, sp.End
		i++
		if sp.NoBreak > 0 {
			for i < len(spans) && spans[i].NoBreak == sp.NoBreak {
				end = spans[i].End
				i++
			}
			if start < at && at < end {
				return start, end, true
			}
		}
		from = end
	}
	return 0, 0, false
}
