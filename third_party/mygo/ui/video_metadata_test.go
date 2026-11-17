package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/internal/media"
)

type audioMetadataEngine struct {
	fakeEngine
	present, known, samples bool
}

func (e *audioMetadataEngine) HasAudio() (bool, bool) { return e.present, e.known }
func (e *audioMetadataEngine) AudioSamples(time.Duration, []float32) (int, bool) {
	return 48000, e.samples
}

func TestVideoAudioTrackMetadata(t *testing.T) {
	e := &audioMetadataEngine{}
	p := NewEngineVideoPlayer(e)
	t.Cleanup(p.Close)
	check := func(present, known bool) {
		t.Helper()
		if got, ready := p.HasAudio(); got != present || ready != known {
			t.Fatalf("HasAudio = (%v, %v), want (%v, %v)", got, ready, present, known)
		}
	}
	check(false, false)
	e.known = true
	check(false, true)
	e.present = true
	for _, state := range []media.State{media.Loading, media.Ready, media.Buffering, media.Ended} {
		e.state = state
		for _, samples := range []bool{false, true} {
			e.samples = samples
			p.AudioSamples(make([]float32, 32))
			p.SetMuted(true)
			p.SetVolume(0)
			p.Pause()
			check(true, true)
			p.SetMuted(false)
			p.SetVolume(1)
			p.Play()
			check(true, true)
		}
	}
	p.Close()
	check(false, false)
}

func TestVideoAudioTrackUnsupportedAndImageDecoder(t *testing.T) {
	p := NewEngineVideoPlayer(&fakeEngine{})
	if has, known := p.HasAudio(); has || known {
		t.Fatal("an engine without track metadata reported a known audio state")
	}
	p.Close()
	p = newVideo(t, videoRed)
	if has, known := p.HasAudio(); has || !known {
		t.Fatal("an image-only decoder did not report a known absence of audio")
	}
	p.Close()
	if has, known := p.HasAudio(); has || known {
		t.Fatal("a closed image player retained media metadata")
	}
}
