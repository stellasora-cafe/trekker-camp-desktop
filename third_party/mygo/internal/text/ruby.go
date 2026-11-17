package text

import "math"

// TypographicMetrics returns the normalized em box used for ruby placement.
// Fonts without OS/2 metrics retain their ascent/descent ratio within one em.
func (s *System) TypographicMetrics(style Style) (ascent, descent float32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.font(style)
	if f == nil {
		return style.FontSize() * .8, style.FontSize() * .2
	}
	return f.TypographicMetrics()
}

func (f *Font) TypographicMetrics() (ascent, descent float32) {
	if f.TypoAscent > 0 {
		return f.TypoAscent, f.TypoDescent
	}
	if height := f.Ascent + f.Descent; height > 0 {
		return f.Size * f.Ascent / height, f.Size * f.Descent / height
	}
	return f.Size * .8, f.Size * .2
}

// An annotation may use the preceding line's space below its em boxes. Keep
// the original glyph indices and baseline shifts while tightening that gap.
func tightenRubyLine(line *Line, previous Line, fontSize float32) float32 {
	if line.rubyExtra <= 0 {
		return 0
	}
	scale := max(line.rubyScale, 1)
	under := previous.Y + (previous.Height+fontSize)/2
	if len(previous.Glyphs) > 0 {
		under = previous.Y
		for _, glyph := range previous.Glyphs {
			if glyph.Object {
				under = max(under, previous.Baseline+previous.Descent)
				continue
			}
			if glyph.Font != nil {
				_, descent := glyph.Font.TypographicMetrics()
				descent = float32(math.Ceil(float64(descent*scale))) / scale
				under = max(under, glyph.Y+descent)
			}
		}
	}
	shift := min(line.rubyExtra, max(float32(0), previous.Y+previous.Height-under))
	line.Ascent -= shift
	line.Height -= shift
	line.Baseline -= shift
	for i := range line.Glyphs {
		line.Glyphs[i].Y -= shift
	}
	return shift
}
