package video

import (
	_ "embed"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/wait"
	"github.com/egoist/mygo/ui"
)

// The same pictures as rgb.mp4, with its silent AAC track removed:
// ffmpeg -i rgb.mp4 -map 0:v:0 -c copy -an rgb-no-audio.mp4
//
//go:embed testdata/rgb-no-audio.mp4
var rgbNoAudioMP4 []byte

func TestAudioTrackMetadata(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
		has  bool
	}{{"silent_aac", rgbMP4, true}, {"no_audio", rgbNoAudioMP4, false}} {
		for _, fromFile := range []bool{false, true} {
			mode := "memory"
			if fromFile {
				mode = "file"
			}
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				p, tt, _ := played(t, func() (*ui.VideoPlayer, error) {
					if !fromFile {
						return OpenData(tc.data)
					}
					path := filepath.Join(t.TempDir(), "音轨 metadata.mp4")
					if err := os.WriteFile(path, tc.data, 0600); err != nil {
						t.Fatal(err)
					}
					return Open(path)
				})
				wait.Until(t, "audio-track metadata", func() bool {
					tt.Frame()
					_, known := p.HasAudio()
					return known
				})
				check := func() {
					t.Helper()
					if has, known := p.HasAudio(); has != tc.has || !known {
						t.Fatalf("HasAudio = (%v, %v), want (%v, true)", has, known, tc.has)
					}
				}
				check()
				p.Pause()
				p.SetMuted(true)
				p.SetVolume(0)
				check()
				p.Seek(500 * time.Millisecond)
				wait.Until(t, "paused seek with track metadata", func() bool {
					tt.Frame()
					check()
					return near(tt.Image().RGBAAt(32, 24), green)
				})
				p.SetMuted(false)
				check()
				p.Close()
				if has, known := p.HasAudio(); has || known {
					t.Fatal("closed media retained audio-track metadata")
				}
			})
		}
	}
}

func TestAudioTrackMetadataWaitsForResource(t *testing.T) {
	ready := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(ready) }) }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-ready:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write(rgbNoAudioMP4)
	}))
	defer srv.Close()
	defer release()
	e, err := newEngine(source{url: srv.URL + "/track.mp4"})
	if errors.Is(err, ErrUnsupported) {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	p := ui.NewEngineVideoPlayer(e)
	defer p.Close()
	var wakes atomic.Int32
	e.SetWake(func() { wakes.Add(1) })
	p.Pause()
	p.SetMuted(true)
	if has, known := p.HasAudio(); has || known {
		t.Fatal("unloaded resource was classified as having no audio")
	}
	release()
	wait.Until(t, "resource metadata", func() bool {
		if _, err := e.State(); err != nil {
			t.Fatal(err)
		}
		has, known := p.HasAudio()
		if known && has {
			t.Fatal("video without audio reported an audio track")
		}
		return known && wakes.Load() > 0
	})
}
