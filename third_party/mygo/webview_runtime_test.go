package mygo

import (
	"runtime"
	"testing"
)

func TestCheckWebViewRuntimeRejectsMissingFixedRuntime(t *testing.T) {
	if runtime.GOOS != "windows" { t.Skip("Windows runtime detection") }
	t.Setenv("WEBVIEW2_BROWSER_EXECUTABLE_FOLDER",t.TempDir())
	if CheckWebViewRuntime()==nil { t.Fatal("missing runtime accepted") }
}
