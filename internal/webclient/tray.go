// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	_ "embed"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

//go:embed tray-icon.png
var trayIcon []byte

func (c *client) configureTrayLifecycle() {
	mygo.App.OnSecondInstance(func([]string, string) { c.restoreFromTray() })
	mygo.App.OnWindowAllClosed(mygo.App.Quit)
	if runtime.GOOS != "windows" {
		return
	}
	_ = c.ensureTray()
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { c.quitting = true })
	mygo.App.OnQuitCanceled(func() { c.quitting = false })
	mygo.App.OnQuit(func() {
		c.stopTray()
		c.cancel()
	})
	mygo.App.OnNotificationClick(func(id string) {
		if strings.HasPrefix(id, trayNoticePrefix) {
			c.restoreFromTray()
		}
	})
}

func (c *client) attachTrayClose(window *mygo.Window) {
	if runtime.GOOS != "windows" {
		return
	}
	window.OnClose(func(event *mygo.CloseEvent) {
		if c.quitting {
			return
		}
		event.PreventDefault()
		if !c.trayClosing {
			c.closeMainWindow(window)
		}
	})
}

func (c *client) isCurrentMain(window *mygo.Window) bool {
	c.windowMu.Lock()
	defer c.windowMu.Unlock()
	return c.window == window && !c.changingWindow && !c.quitting && !window.IsDestroyed() && c.ctx.Err() == nil
}

func (c *client) closeMainWindow(window *mygo.Window) {
	if !c.isCurrentMain(window) {
		return
	}
	switch c.closeBehavior.get() {
	case closeAsk:
		c.showCloseDialog(window, "关闭按钮行为", "收进托盘后仍可接收通知和心链消息。选择会自动保存，可在偏好设置中更改。", func(action closeBehavior) {
			if !c.isCurrentMain(window) {
				return
			}
			if err := c.closeBehavior.set(action); err != nil {
				c.showCloseDialog(window, "暂时无法保存", "无法保存关闭按钮行为，请重试。", nil)
				return
			}
			c.closeMainWindow(window)
		})
	case closeQuit:
		c.quitFromWindow(window)
	case closeTray:
		c.hideToTray(window)
	}
}

func (c *client) quitFromWindow(window *mygo.Window) {
	if c.trayClosing {
		return
	}
	c.trayClosing = true
	go func() {
		safe := c.prepareWindowExit(true, true)
		mygo.RunOnMain(func() {
			c.trayClosing = false
			if !c.isCurrentMain(window) {
				return
			}
			if safe {
				mygo.App.Quit()
				return
			}
			c.restoreFromTray()
			c.showCloseDialog(window, "暂时无法关闭", "请先完成当前编辑、上传或弹窗操作，再关闭窗口。", nil)
		})
	}()
}

func (c *client) showCloseDialog(parent *mygo.Window, title, detail string, choose func(closeBehavior)) {
	c.trayClosing = true
	var dialog *mygo.Window
	finish := func(action closeBehavior) {
		dialog.Destroy()
		if choose != nil && action != closeAsk {
			choose(action)
		}
	}
	dialog = mygo.NewWindow(mygo.WindowOptions{
		Parent: parent, Modal: true, Title: title, Width: 456, Height: 224,
		UseContentSize: true, Frameless: true, CornerRadius: 20, SkipTaskbar: true,
		DisableResize: true, DisableMinimize: true, DisableMaximize: true, DisableFullScreen: true,
		BackgroundColor: "light-dark(#ffffff,#252733)",
		Content: ui.View(func(ctx *ui.Context) {
			t := permissionTheme(ctx.Theme().Dark)
			ctx.OnWindowShortcut(0, ui.KeyEscape, func() { finish(closeAsk) })
			ui.Themed(ctx, t).Fill().Padding(20, 24, 22, 24).Gap(14).Background(t.Background).DragWindow().
				Role(ui.RoleDialog).Label(title).Children(func() {
				ui.Row(ctx).AlignItems(ui.Center).Gap(12).FillWidth().Children(func() {
					ui.Box(ctx).Size(42, 42).Radius(14).Background(t.Accent.Alpha(.12)).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
						ui.Icon(ctx, permissionIcons[mygo.PermissionNotifications]).Size(22, 22).TextColor(t.Accent)
					})
					ui.Text(ctx, title).FontSize(20).Bold().Unselectable()
				})
				ui.Text(ctx, detail).FillWidth().FontSize(14).TextColor(t.Text).Unselectable()
				ui.Spacer(ctx)
				ui.Row(ctx).FillWidth().Justify(ui.End).Gap(8).Children(func() {
					if choose == nil {
						ui.PrimaryButton(ctx.Key("confirm"), "知道了").Height(38).OnClick(func() { finish(closeAsk) })
						return
					}
					ui.Button(ctx.Key("cancel"), "取消").Height(38).AutoFocus().OnClick(func() { finish(closeAsk) })
					ui.Button(ctx.Key("quit"), "退出应用").Height(38).OnClick(func() { finish(closeQuit) })
					ui.PrimaryButton(ctx.Key("tray"), "收进托盘").Height(38).OnClick(func() { finish(closeTray) })
				})
				nativeCaptionControl(ctx, t, "close", "关闭弹窗", campCloseIcon, func() { finish(closeAsk) }).Absolute().Right(0).Top(0)
			})
		}),
	})
	dialog.OnClosed(func() { c.trayClosing = false })
}

func (c *client) hideToTray(window *mygo.Window) {
	if c.inTray.Load() {
		return
	}
	if err := c.ensureTray(); err != nil {
		c.showCloseDialog(window, "暂时无法收起", "未能创建托盘图标，请重试。", nil)
		return
	}
	c.inTray.Store(true)
	c.trayWindows = make(map[*mygo.Window]bool)
	var visible []*mygo.Window
	for _, open := range mygo.Windows() {
		if open.IsVisible() {
			visible = append(visible, open)
		}
	}
	for _, open := range visible {
		c.hideTrayWindow(open)
	}
}

func (c *client) hideTrayWindow(window *mygo.Window) {
	if _, ok := c.trayWindows[window]; ok {
		return
	}
	page := window.Page()
	mute := page != nil && !page.IsAudioMuted()
	if mute {
		page.SetAudioMuted(true)
	}
	c.trayWindows[window] = mute
	window.Hide()
}

func (c *client) showReadyWindow(window *mygo.Window) {
	mygo.RunOnMain(func() {
		if window.IsDestroyed() {
			return
		}
		if c.inTray.Load() {
			c.hideTrayWindow(window)
			return
		}
		if !window.IsVisible() {
			window.Show()
		}
	})
}

func (c *client) ensureTray() error {
	if c.tray != nil {
		return nil
	}
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Label: "打开旅人营地", Click: func(*mygo.MenuItem, *mygo.Window) { c.restoreFromTray() }},
		mygo.Separator(),
		{Label: "退出旅人营地", Click: func(*mygo.MenuItem, *mygo.Window) {
			c.windowMu.Lock()
			window := c.window
			c.windowMu.Unlock()
			if window != nil {
				c.quitFromWindow(window)
			}
		}},
	})
	tray, err := mygo.NewTray(mygo.TrayOptions{ID: "camp", Icon: trayIcon, ToolTip: "旅人营地", Menu: menu})
	if err != nil {
		return err
	}
	c.tray = tray
	tray.OnClick(c.restoreFromTray)
	return nil
}

func (c *client) stopTray() {
	if c.tray != nil {
		c.tray.Destroy()
		c.tray = nil
	}
}

func (c *client) restoreFromTray() {
	mygo.RunOnMain(func() {
		if c.quitting {
			return
		}
		c.windowMu.Lock()
		window := c.window
		c.windowMu.Unlock()
		if c.inTray.Swap(false) {
			for open, unmute := range c.trayWindows {
				if open.IsDestroyed() {
					continue
				}
				if unmute {
					open.Page().SetAudioMuted(false)
				}
				if open != window {
					open.ShowInactive()
				}
			}
			c.trayWindows = nil
		}
		if window != nil && !window.IsDestroyed() {
			window.Show()
			if window.IsMinimized() {
				window.Restore()
			}
			window.Focus()
			c.showPermissionPrompt()
		}
	})
}
