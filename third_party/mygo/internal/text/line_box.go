package text

func (span Span) exactLineBox() bool {
	return span.ExactLineBox && (span.LineAscent > 0 || span.LineDescent > 0)
}

// exactLineBoxRange requires coverage of the entire range, including the
// font's runs shared by differently styled spans. Unstyled characters keep
// the paragraph's original font strut and fallback-font extents.
func exactLineBoxRange(spans []Span, start, end int) bool {
	if end <= start {
		return false
	}
	at := 0
	for _, span := range spans {
		if span.End <= start {
			at = span.End
			continue
		}
		if at >= end {
			break
		}
		if !span.exactLineBox() {
			return false
		}
		if span.End >= end {
			return true
		}
		at = span.End
	}
	return false
}
