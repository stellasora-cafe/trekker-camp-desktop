//go:build js

package video

import "syscall/js"

func (e *elementEngine) HasAudio() (present, known bool) {
	if e.closed || e.el.Get("readyState").Int() < 1 { // HAVE_METADATA
		return false, false
	}
	if tracks := e.el.Get("audioTracks"); !tracks.IsUndefined() && !tracks.IsNull() {
		return tracks.Get("length").Int() > 0, true
	}
	if has := e.el.Get("mozHasAudio"); has.Type() == js.TypeBoolean {
		return has.Bool(), true
	}
	// Browsers without track metadata must not mistake zero decoded bytes,
	// muted playback, or an unavailable analyser for an absent audio track.
	return false, false
}
