package mygo

import (
	"github.com/egoist/mygo/internal/webview2"
	"runtime"
)

// CheckWebViewRuntime checks the same runtime files used by the Windows backend,
// without creating a browser or requiring an initialized application.
func CheckWebViewRuntime() error {
	if runtime.GOOS != "windows" {
		return nil
	}
	_, _, err := webview2.Find()
	return err
}
