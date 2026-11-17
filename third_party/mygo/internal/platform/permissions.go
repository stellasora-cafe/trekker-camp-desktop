package platform

// PermissionRequests keeps native requests alive until answered or the page
// closes. All methods and the returned response functions run on the main thread.
type PermissionRequests map[*bool]func(bool)

// Add returns a response that completes the request at most once, including
// when a dialog responds after Cancel already denied the request.
func (p *PermissionRequests) Add(respond func(bool)) func(bool) {
	if *p == nil {
		*p = PermissionRequests{}
	}
	done := new(bool)
	complete := func(allow bool) {
		if *done {
			return
		}
		*done = true
		delete(*p, done)
		respond(allow)
	}
	(*p)[done] = complete
	return complete
}

// Cancel denies outstanding requests before their native view is released.
func (p PermissionRequests) Cancel() {
	for _, respond := range p {
		respond(false)
	}
}
