//go:build linux && (amd64 || arm64)

package video

func (e *gstEngine) HasAudio() (present, known bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed || !e.prerolled {
		return false, false
	}
	var v gValue
	gValueInit(&v, gTypeInt)
	gObjectGetProperty(e.pipe, "n-audio", &v)
	n := gValueGetInt(&v)
	gValueUnset(&v)
	return n > 0, true
}
