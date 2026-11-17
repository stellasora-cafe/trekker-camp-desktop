package ui

// HasAudio reports whether the media contains an audio track. known is false
// until its track metadata has loaded, or when the engine cannot report it.
// An audio track stays present when muted, at zero volume, paused, or silent;
// this does not depend on AudioSamples being available. Image-only decoders,
// such as GIF and APNG, report (false, true). A closed player reports unknown.
// Like the player's other methods, call it on the UI thread.
func (p *VideoPlayer) HasAudio() (present, known bool) {
	if p.closed {
		return false, false
	}
	if p.eng == nil {
		return false, p.dec != nil
	}
	if p.eng.closed {
		return false, false
	}
	if e, ok := p.eng.e.(interface{ HasAudio() (bool, bool) }); ok {
		return e.HasAudio()
	}
	return false, false
}
