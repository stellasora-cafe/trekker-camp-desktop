package video

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/wait"
	"github.com/egoist/mygo/ui"
)

func silentWAV() []byte {
	const samples = 8000
	wav := make([]byte, 44+samples*2)
	copy(wav, "RIFF")
	binary.LittleEndian.PutUint32(wav[4:], uint32(len(wav)-8))
	copy(wav[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(wav[16:], 16)
	binary.LittleEndian.PutUint16(wav[20:], 1)
	binary.LittleEndian.PutUint16(wav[22:], 1)
	binary.LittleEndian.PutUint32(wav[24:], 8000)
	binary.LittleEndian.PutUint32(wav[28:], 16000)
	binary.LittleEndian.PutUint16(wav[32:], 2)
	binary.LittleEndian.PutUint16(wav[34:], 16)
	copy(wav[36:], "data")
	binary.LittleEndian.PutUint32(wav[40:], samples*2)
	return wav
}

func TestMediaFoundationLocalFilenames(t *testing.T) {
	for _, name := range []string{"audio.wav", "本地 音频.wav", "百分% #音频.wav"} {
		path := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(path, silentWAV(), 0600); err != nil {
			t.Fatal(err)
		}
		for _, mode := range []string{"path", "file_url"} {
			t.Run(name+"/"+mode, func(t *testing.T) {
				var p *ui.VideoPlayer
				var err error
				if mode == "path" {
					p, err = Open(path)
				} else {
					p, err = OpenURL(fileURL(path))
				}
				if err != nil {
					t.Fatal(err)
				}
				defer p.Close()
				p.SetMuted(true)
				tt := ui.NewTester(func(c *ui.Context) { ui.Video(c, p).Size(40, 40) }, 40, 40)
				wait.Until(t, "local audio metadata", func() bool {
					tt.Frame()
					if err := p.Err(); err != nil {
						t.Fatal(err)
					}
					return p.Duration() > 0
				})
				if p.Duration() != time.Second {
					t.Fatalf("duration=%v", p.Duration())
				}
			})
		}
	}
}

func TestMediaFoundationFileURL(t *testing.T) {
	for _, tt := range []struct{ input, want string }{
		{"file:///C:/%E4%B8%AD%E6%96%87/a%20b%23%25.wav", `C:\中文\a b#%.wav`},
		{"file://server/share/a%20b.wav", `\\server\share\a b.wav`},
		{"file://localhost/C:/a%20b.wav", `C:\a b.wav`},
		{"https://localhost/media/a%20b.wav", "https://localhost/media/a%20b.wav"},
		{`C:\中文\a b.wav`, `C:\中文\a b.wav`},
	} {
		if got := mediaFoundationSource(tt.input); got != tt.want {
			t.Errorf("%q => %q, want %q", tt.input, got, tt.want)
		}
	}
}
