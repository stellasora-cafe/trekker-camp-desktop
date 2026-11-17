//go:build windows && (amd64 || arm64)

package video

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/wait"
	"github.com/egoist/mygo/ui"
)

func TestMediaFoundationInitialPicture(t *testing.T) {
	for _, mode := range []string{"memory", "file"} {
		for _, seek := range []bool{false, true} {
			name := mode
			if seek {
				name += "/initial_seek"
			} else {
				name += "/paused"
			}
			t.Run(name, func(t *testing.T) {
				var p *ui.VideoPlayer
				var err error
				if mode == "memory" {
					p, err = OpenData(rgbMP4)
				} else {
					path := filepath.Join(t.TempDir(), "本地 预览.mp4")
					if err := os.WriteFile(path, rgbMP4, 0600); err != nil {
						t.Fatal(err)
					}
					p, err = Open(path)
				}
				if err != nil {
					t.Fatal(err)
				}
				defer p.Close()
				p.SetMuted(true)
				p.Pause()
				want, at := red, time.Duration(0)
				if seek {
					want, at = blue, 900*time.Millisecond
					p.Seek(at)
				}
				tt := ui.NewTester(func(c *ui.Context) { ui.Video(c, p).Size(64, 48) }, 64, 48)
				wait.Until(t, "the first positioned picture", func() bool {
					tt.Frame()
					if err := p.Err(); err != nil {
						t.Fatal(err)
					}
					return p.Ready()
				})
				if got := tt.Image().RGBAAt(32, 24); !near(got, want) {
					t.Fatalf("initial frame=%v, want %v", got, want)
				}
				if p.Playing() || p.Position() < at-100*time.Millisecond || p.Position() > at+100*time.Millisecond {
					t.Fatalf("paused=%v position=%v, want %v", !p.Playing(), p.Position(), at)
				}
				p.Seek(100 * time.Millisecond)
				wait.Until(t, "paused memory/file seek", func() bool { tt.Frame(); return near(tt.Image().RGBAAt(32, 24), red) })
				if p.Playing() {
					t.Fatal("a seek started paused playback")
				}
				p.Play()
				wait.Until(t, "playback to completion", func() bool { tt.Frame(); return p.Ended() })
				if got := tt.Image().RGBAAt(32, 24); !near(got, blue) {
					t.Fatalf("last frame=%v", got)
				}
			})
		}
	}
}

func TestMediaFoundationMemoryAudio(t *testing.T) {
	p, err := OpenData(silentWAV())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	p.SetMuted(true)
	tt := ui.NewTester(func(c *ui.Context) { ui.Video(c, p).Size(40, 40) }, 40, 40)
	wait.Until(t, "memory audio playback", func() bool {
		tt.Frame()
		if err := p.Err(); err != nil {
			t.Fatal(err)
		}
		return p.Ended()
	})
	rate, available := p.AudioSamples(make([]float32, 4096))
	if !available || rate <= 0 {
		t.Fatal("8 kHz source did not negotiate an analysis format")
	}
	if p.Duration() != time.Second || p.Ready() {
		t.Fatal("audio metadata or video readiness incorrect")
	}
	if has, known := p.HasAudio(); !has || !known {
		t.Fatal("a silent, muted audio-only file lost its audio-track metadata")
	}
}
