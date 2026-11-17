//go:build js

package video

import (
	"syscall/js"
	"testing"
)

func TestElementAudioTrackMetadata(t *testing.T) {
	for _, tc := range []struct {
		name           string
		properties     map[string]any
		present, known bool
	}{
		{"unloaded", map[string]any{"readyState": 0, "audioTracks": map[string]any{"length": 0}}, false, false},
		{"no_audio", map[string]any{"readyState": 1, "audioTracks": map[string]any{"length": 0}}, false, true},
		{"muted", map[string]any{"readyState": 1, "muted": true, "volume": 0, "audioTracks": map[string]any{"length": 1}}, true, true},
		{"firefox_audio", map[string]any{"readyState": 1, "mozHasAudio": true}, true, true},
		{"firefox_no_audio", map[string]any{"readyState": 1, "mozHasAudio": false}, false, true},
		{"unknown", map[string]any{"readyState": 4, "webkitAudioDecodedByteCount": 0, "muted": true}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &elementEngine{el: js.ValueOf(tc.properties)}
			if has, known := e.HasAudio(); has != tc.present || known != tc.known {
				t.Fatalf("HasAudio = (%v, %v), want (%v, %v)", has, known, tc.present, tc.known)
			}
			e.closed = true
			if has, known := e.HasAudio(); has || known {
				t.Fatal("closed element retained metadata")
			}
		})
	}
}
