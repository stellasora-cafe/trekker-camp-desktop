package ui

// TextLineCount reports this paragraph's last completed layout. Truncated is
// true when MaxLines or SingleLine omitted text. Before its first layout both
// values are zero. Read it in DrawOver to observe the current frame's result.
func (e *node) TextLineCount() (count int, truncated bool) {
	if e.tl == nil {
		return 0, false
	}
	count = len(e.tl.Lines)
	if count > 0 {
		truncated = e.tl.Lines[count-1].End < len(e.tl.Runes)
	}
	return
}
