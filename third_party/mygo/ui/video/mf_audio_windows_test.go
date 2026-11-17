//go:build windows && (amd64 || arm64)

package video

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/wait"
	"github.com/egoist/mygo/ui"
)

func analysisToneWAV() []byte {
	const rate = 48000
	data := make([]byte, 44+rate*2*2)
	copy(data, silentWAV()[:44])
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	binary.LittleEndian.PutUint32(data[24:], rate)
	binary.LittleEndian.PutUint32(data[28:], rate*2)
	binary.LittleEndian.PutUint32(data[40:], uint32(len(data)-44))
	for i := 0; i < rate*2; i++ {
		freq := 220.
		if i >= rate {
			freq = 880
		}
		v := int16(math.Sin(2*math.Pi*freq*float64(i)/rate) * 16000)
		binary.LittleEndian.PutUint16(data[44+i*2:], uint16(v))
	}
	return data
}

func TestMediaFoundationAudioAnalysis(t *testing.T) {
	for _, source := range []string{"memory", "unicode_file"} {
		t.Run(source, func(t *testing.T) {
			var p *ui.VideoPlayer
			var err error
			if source == "memory" {
				p, err = OpenData(analysisToneWAV())
			} else {
				path := filepath.Join(t.TempDir(), "频谱 音频.wav")
				if err := os.WriteFile(path, analysisToneWAV(), 0600); err != nil {
					t.Fatal(err)
				}
				p, err = Open(path)
			}
			if err != nil {
				t.Fatal(err)
			}
			defer p.Close()
			// The analyser must observe PCM upstream of both mute and volume.
			p.SetMuted(true)
			p.SetVolume(0)
			tt := ui.NewTester(func(c *ui.Context) { ui.Video(c, p).Size(8, 8) }, 8, 8)
			samples := make([]float32, 4096)
			check := func(want float64) {
				wait.Until(t, "positioned audio samples", func() bool {
					tt.Frame()
					if p.Err() != nil {
						t.Fatal(p.Err())
					}
					rate, ok := p.AudioSamples(samples)
					if !ok {
						return false
					}
					var crossings int
					var peak float32
					for i, v := range samples {
						peak = max(peak, float32(math.Abs(float64(v))))
						if i > 0 && samples[i-1] <= 0 && v > 0 {
							crossings++
						}
					}
					frequency := float64(crossings) * float64(rate) / float64(len(samples))
					return peak > .3 && math.Abs(frequency-want) < 20
				})
			}
			wait.Until(t, "audio playhead", func() bool {
				tt.Frame()
				if p.Err() != nil {
					t.Fatal(p.Err())
				}
				return p.Position() > 300*time.Millisecond
			})
			check(220)
			ranges := p.Buffered()
			if len(ranges) == 0 || ranges[len(ranges)-1].End < 1900*time.Millisecond {
				t.Fatalf("buffered memory/file ranges=%v", ranges)
			}
			p.Seek(1300 * time.Millisecond)
			check(880)
			p.Pause()
			position := p.Position()
			tt.Advance(50 * time.Millisecond)
			if p.Position() > position+30*time.Millisecond {
				t.Fatal("pause advanced audio")
			}
		})
	}
}

func TestAudioPCMConversions(t *testing.T) {
	for _, bits := range []int{8, 16, 24, 32} {
		data := make([]byte, bits/8*2)
		if bits == 8 {
			data[0] = 192
			data[1] = 64
		} else {
			data[bits/8-1] = 0x40
			data[bits/8*2-1] = 0xc0
		}
		got := decodeAudioPCM(data, audioPCMFormat{channels: 1, bits: bits})
		if len(got) != 2 || got[0] != .5 || got[1] != -.5 {
			t.Fatalf("PCM%d=%v", bits, got)
		}
	}
	data := make([]byte, 8)
	binary.LittleEndian.PutUint32(data, math.Float32bits(.25))
	binary.LittleEndian.PutUint32(data[4:], math.Float32bits(.75))
	if got := decodeAudioPCM(data, audioPCMFormat{channels: 2, bits: 32, floating: true}); len(got) != 1 || got[0] != .5 {
		t.Fatalf("stereo float=%v", got)
	}
	data = make([]byte, 24)
	for i, v := range []float32{.1, .2, .3, 1, .4, .5} {
		binary.LittleEndian.PutUint32(data[i*4:], math.Float32bits(v))
	}
	want := float32(math.Sqrt(.5))*.3 + .3 + .5*.9
	if got := decodeAudioPCM(data, audioPCMFormat{channels: 6, bits: 32, floating: true}); len(got) != 1 || math.Abs(float64(got[0]-want)) > 1e-7 {
		t.Fatalf("5.1 speaker downmix=%v want%v", got, want)
	}
}
