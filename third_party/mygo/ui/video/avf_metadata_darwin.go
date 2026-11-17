//go:build darwin

package video

func (e *avEngine) HasAudio() (present, known bool) {
	defer pooled()()
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed || msgInt(e.item, sel("status")) != avStatusReadyToPlay {
		return false, false
	}
	tracks := send(e.item, "tracks")
	known = true
	for i, n := 0, int(send(tracks, "count")); i < n; i++ {
		track := send(send(tracks, "objectAtIndex:", i), "assetTrack")
		if track == 0 {
			known = false
			continue
		}
		if typ := send(track, "mediaType"); typ != 0 && cfEqual(uintptr(typ), avMediaTypeAudio) {
			return true, true
		}
	}
	return false, known
}
