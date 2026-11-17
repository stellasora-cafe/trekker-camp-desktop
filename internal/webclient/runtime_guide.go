// SPDX-FileCopyrightText: stellasora-cafe contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const webViewDownloadURL = "https://developer.microsoft.com/microsoft-edge/webview2/#download"

// ShowRuntimeGuide opens a native-only window when browser pages cannot run.
// It is safe before any session or WebView has been initialized.
func ShowRuntimeGuide() bool {
	if mygo.CheckWebViewRuntime() == nil {
		return false
	}
	var window *mygo.Window
	message := "旅人营地需要 Microsoft Edge WebView2 Runtime。请前往微软官网安装 Evergreen Runtime，完成后重新启动客户端。"
	window = mygo.NewWindow(mygo.WindowOptions{
		Title: "安装 WebView2", Width: 480, Height: 420, UseContentSize: true,
		Frameless: true, DisableResize: true, DisableMaximize: true,
		BackgroundColor: "light-dark(#ffffff,#252733)",
		Content: ui.View(func(ctx *ui.Context) {
			t := permissionTheme(ctx.Theme().Dark)
			ui.Themed(ctx, t).Fill().Padding(windowCaptionHeight+8, 28, 24, 28).Gap(18).Background(t.Background).Children(func() {
				ui.Text(ctx, "需要安装 WebView2 运行时").FontSize(21)
				ui.Text(ctx, message).FillWidth().FontSize(14)
				ui.Spacer(ctx)
				ui.PrimaryButton(ctx, "前往微软官网下载").FillWidth().Height(42).OnClick(func() {
					if err := mygo.Shell.OpenExternal(webViewDownloadURL); err != nil {
						message = "无法打开浏览器，请访问：" + webViewDownloadURL
						window.Invalidate()
					}
				})
				ui.Button(ctx, "安装完成，重新启动").FillWidth().Height(42).OnClick(func() {
					if mygo.CheckWebViewRuntime() != nil {
						message = "仍未检测到可用的 WebView2 运行时。请完成微软安装程序后重试。"
						window.Invalidate()
						return
					}
					mygo.App.Relaunch()
				})
				ui.Box(ctx).Absolute().Left(0).Right(42).Top(0).Height(windowCaptionHeight).DragWindow()
				nativeCaptionControl(ctx, t, "close", "关闭", permissionCloseIcon, window.Close).Absolute().Right(0).Top(0)
			})
		}),
	})
	window.Show()
	return true
}
