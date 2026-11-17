package mygo

// Permission is something a page asks the user for.
type Permission string

// Permissions.
const (
	PermissionCamera        Permission = "camera"
	PermissionMicrophone    Permission = "microphone"
	PermissionGeolocation   Permission = "geolocation"
	PermissionNotifications Permission = "notifications"
)

// PermissionRequest is passed to the permission handler of a window.
type PermissionRequest struct {
	// Permissions asked for together, such as the camera and the
	// microphone of a video call.
	Permissions []Permission
	// Origin of the page asking, e.g. "https://example.com".
	Origin string
}

// SetPermissionHandler decides what the pages of the window may use, such
// as the camera: fn returns whether to grant a request, and runs on the main
// thread. nil restores the default: the app's own pages (see
// PageOptions.TrustedOrigins) get what they ask for, other pages do not.
//
// The operating system may still ask the user, like macOS does once per
// app for the camera and the microphone. That needs usage descriptions in
// macos.infoPlist of mygo.config.ts (NSCameraUsageDescription,
// NSMicrophoneUsageDescription), without which macOS ends the app.
func (p *Page) SetPermissionHandler(fn func(req PermissionRequest) bool) {
	p.w.mu.Lock()
	p.w.permissionHandler = fn
	p.w.permissionRequestHandler = nil
	p.w.mu.Unlock()
}

// SetPermissionRequestHandler decides permission requests asynchronously.
// fn runs on the main thread and may return while a custom dialog is open.
// Call respond with the user's decision, from any goroutine. Only its first
// call takes effect. Closing the page denies requests still awaiting a reply.
//
// This replaces SetPermissionHandler. nil restores the same default policy.
func (p *Page) SetPermissionRequestHandler(fn func(req PermissionRequest, respond func(bool))) {
	p.w.mu.Lock()
	p.w.permissionRequestHandler = fn
	p.w.permissionHandler = nil
	p.w.mu.Unlock()
}

func (h *windowHandler) PermissionRequested(kinds []string, origin string, respond func(bool)) {
	if h.w.destroyed.Load() {
		respond(false)
		return
	}
	h.w.mu.Lock()
	fn := h.w.permissionHandler
	async := h.w.permissionRequestHandler
	h.w.mu.Unlock()
	if fn == nil && async == nil {
		respond(h.w.isTrusted(origin))
		return
	}
	req := PermissionRequest{Origin: origin}
	for _, k := range kinds {
		req.Permissions = append(req.Permissions, Permission(k))
	}
	if async != nil {
		done := false // accessed only on the main thread
		async(req, func(allow bool) {
			onMain(func() {
				if !done {
					done = true
					respond(allow && !h.w.destroyed.Load())
				}
			})
		})
		return
	}
	respond(fn(req) && !h.w.destroyed.Load())
}
