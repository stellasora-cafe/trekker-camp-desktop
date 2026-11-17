//go:build windows && (amd64 || arm64)

package video

const engineHasAudio = 39 // IMFMediaEngine::HasAudio

func (e *mfEngine) HasAudio() (present, known bool) {
	e.mu.Lock()
	known = e.metadata && !e.closed
	e.mu.Unlock()
	if !known {
		return false, false
	}
	// Before LOADEDMETADATA, HasAudio also returns false for an unloaded
	// resource. Query it only once absence has a definite meaning.
	return call(e.engine, engineHasAudio) != 0, true
}
