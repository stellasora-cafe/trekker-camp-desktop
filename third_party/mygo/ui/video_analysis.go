package ui

import (
	"time"

	"github.com/egoist/mygo/internal/media"
)

// MediaTimeRange is an interval of downloaded media, measured from its start.
type MediaTimeRange = media.TimeRange

// Buffered returns the system player's actual downloaded time ranges.
// A nil result means no range is available yet or the engine cannot report it.
func (p *VideoPlayer) Buffered() []MediaTimeRange {
	if p.closed || p.eng == nil || p.eng.closed {
		return nil
	}
	if e, ok := p.eng.e.(interface{ Buffered() []media.TimeRange }); ok {
		return e.Buffered()
	}
	return nil
}

// AudioSamples copies mono PCM samples ending at the current playhead into dst.
// Samples precede the player's volume/mute gain. The return values are their
// sample rate and whether the engine can provide decoded audio for analysis.
// Like the player's other methods, call it on the UI thread.
func (p *VideoPlayer) AudioSamples(dst []float32) (sampleRate int, available bool) {
	if p.closed || p.eng == nil || p.eng.closed {
		return 0, false
	}
	if e, ok := p.eng.e.(interface {
		AudioSamples(time.Duration, []float32) (int, bool)
	}); ok {
		return e.AudioSamples(p.Position(), dst)
	}
	return 0, false
}
