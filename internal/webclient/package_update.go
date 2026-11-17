// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/stellasora-cafe/trekker-camp-desktop/internal/webfrontend"
)

// packageUpdatePrompt is owned by the main thread. Network work reports back
// through change, so a server-required update can promote an existing prompt.
type packageUpdatePrompt struct {
	client                                            *client
	window                                            *mygo.Window
	ctx                                               context.Context
	cancel                                            context.CancelFunc
	required, busy, installed, exiting, closeOnAccept bool
	safetyFailed                                      bool
	minimum, title, detail, label                     string
	update                                            *mygo.Update
}

func (c *client) packageUpdateLoop() {
	if !mygo.Updater.Enabled() {
		return
	}
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-timer.C:
			c.checkPackageUpdate()
			timer.Reset(6 * time.Hour)
		}
	}
}

func (c *client) checkPackageUpdate() {
	if c.inTray.Load() || !mygo.Updater.Enabled() || !c.packageChecking.CompareAndSwap(false, true) {
		return
	}
	ctx, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	update, err := mygo.Updater.Check(ctx)
	cancel()
	if err != nil || update == nil || c.ctx.Err() != nil {
		c.packageChecking.Store(false)
		return
	}
	mygo.RunOnMain(func() {
		if c.packagePrompt != nil {
			c.packageChecking.Store(false)
			return
		}
		p := c.openPackagePrompt(false, "")
		if p == nil {
			c.packageChecking.Store(false)
			return
		}
		p.update = update
		p.title, p.detail, p.label = "发现新版本 "+update.Version, "更新客户端宿主与内置关于页。账号、设置和草稿会保留。", "下载并安装"
		p.window.Invalidate()
	})
}

// checkAllUpdates is the explicit entry point shared by the native welcome
// menu and the website's desktop-only update menu.
func (c *client) checkAllUpdates() {
	mygo.RunOnMain(func() {
		p := c.openPackagePrompt(false, "")
		if p != nil && !p.busy {
			p.check()
		}
	})
}

func (c *client) requirePackageUpdate(minimum string) {
	mygo.RunOnMain(func() {
		if c.inTray.Load() {
			return
		}
		c.packageRequired.Store(true)
		p := c.openPackagePrompt(true, minimum)
		if p == nil {
			return
		}
		if !p.busy {
			p.check()
		}
	})
}

func packageVersionAtLeast(version, minimum string) bool {
	if minimum == "" {
		return true
	}
	parse := func(value string) ([]uint64, bool) {
		parts := strings.Split(value, ".")
		if len(parts) != 3 {
			return nil, false
		}
		result := make([]uint64, 3)
		for i, part := range parts {
			number, err := strconv.ParseUint(part, 10, 64)
			if err != nil || part == "" {
				return nil, false
			}
			result[i] = number
		}
		return result, true
	}
	available, a := parse(version)
	required, b := parse(minimum)
	if !a || !b {
		return false
	}
	for i := range available {
		if available[i] != required[i] {
			return available[i] > required[i]
		}
	}
	return true
}

// Ordinary updates never discard editors. Required updates ask the frontend
// to flush all registered drafts first, and fail closed if it cannot do so.
const packageRestartSafeScript = `(()=>{
if (!document.getElementById('misskey_app')?.childElementCount) return false;
const maintenance = e => e.matches('[data-camp-maintenance-view]') ||
 document.documentElement.dataset.campMaintenance === 'true' && e.matches('dialog') && e.querySelector(':scope > div[tabindex="-1"] > section[aria-live="polite"]');
if ([...document.querySelectorAll('dialog[open],[role="dialog"]')].some(e=>!maintenance(e))) return false;
if (document.querySelector('[data-camp-pending-edit="true"]')) return false;
if ([...document.querySelectorAll('input[type="file"]')].some(e=>e.files?.length)) return false;
return ![...document.querySelectorAll('textarea,[contenteditable="true"]')].some(e=>e.isContentEditable?e.textContent?.trim():e.value?.trim());
})()`

func (c *client) packageRestartSafe() bool { return c.preparePackageRestart(false) }

func (c *client) preparePackageRestart(required bool) bool {
	return c.prepareWindowExit(required, false)
}

func (c *client) prepareWindowExit(saveDrafts, closing bool) bool {
	c.windowMu.Lock()
	if c.changingWindow || c.window == nil {
		c.windowMu.Unlock()
		return false
	}
	main, guest := c.window, c.guest
	windows := make([]*mygo.Window, 0, len(c.tools)+1)
	if !guest && main.Page() != nil {
		windows = append(windows, main)
	}
	for window := range c.tools {
		windows = append(windows, window)
	}
	c.windowMu.Unlock()
	if main.IsDestroyed() {
		return false
	}
	for _, window := range windows {
		if window.IsDestroyed() {
			continue
		}
		page := window.Page()
		// The standalone maintenance document has no frontend editors.
		if closing && window == main && webfrontend.IsMaintenanceURL(c.origin, page.URL()) {
			continue
		}
		if PackagePageRestartSafe(c.ctx, page, saveDrafts) {
			continue
		}
		// Preference toggles commit immediately. The package updater's generic
		// defaultChecked comparison sees their stored values as edited inputs.
		// On close, still check the page for text editors, uploads and dialogs.
		if closing && window == main {
			// Cached frontends may still count their maintenance dialog as an
			// editor. Keep all unsaved inputs protected in this host fallback.
			if c.maintenanceState.Load() && sameOrigin(c.origin, page.URL()) && maintenancePageCloseSafe(c.ctx, page) {
				continue
			}
			if raw := page.URL(); sameOrigin(c.origin, raw) {
				target, err := url.Parse(raw)
				if err == nil && target.Path == "/settings/preferences" && preferencesPageCloseSafe(c.ctx, page) {
					continue
				}
			}
		}
		return false
	}
	return true
}

const maintenanceCloseSafeScript = `(` + packageRestartSafeScript + `) &&
 ![...document.querySelectorAll('input')].some(e=>e.files?.length||e.value!==e.defaultValue||e.checked!==e.defaultChecked) &&
 ![...document.querySelectorAll('select')].some(e=>[...e.options].some(o=>o.selected!==o.defaultSelected))`

func maintenancePageCloseSafe(ctx context.Context, page *mygo.Page) bool {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	result, err := page.EvalContext(ctx, maintenanceCloseSafeScript)
	return err == nil && result == true
}

func preferencesPageCloseSafe(ctx context.Context, page *mygo.Page) bool {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	// Preserve ordinary unsaved input. Checked values of live switches and
	// radios on this page need not match their HTML defaults.
	result, err := page.EvalContext(ctx, `(`+packageRestartSafeScript+`) && ![...document.querySelectorAll('input')].some(
e=>e.type!=='checkbox'&&e.type!=='radio'&&e.type!=='range'&&e.value!==e.defaultValue)`)
	return err == nil && result == true
}

func PackagePageRestartSafe(ctx context.Context, page *mygo.Page, required bool) bool {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	script := packageRestartSafeScript
	if required {
		script = `(async()=>typeof window.__campPreparePackageRestart==='function'?await window.__campPreparePackageRestart()===true:` + packageRestartSafeScript + `)()`
	}
	result, err := page.EvalContext(ctx, script)
	return err == nil && result == true
}

func (c *client) openPackagePrompt(required bool, minimum string) *packageUpdatePrompt {
	if c.inTray.Load() {
		return nil
	}
	required = required || c.packageRequired.Load()
	if existing := c.packagePrompt; existing != nil && !existing.window.IsDestroyed() {
		if required {
			existing.required, existing.minimum, existing.closeOnAccept = true, minimum, false
			existing.title = "需要更新客户端"
			if !existing.busy {
				existing.label = "检查更新"
			}
		}
		existing.window.Show()
		existing.window.Focus()
		existing.window.Invalidate()
		return existing
	}
	c.windowMu.Lock()
	parent := c.window
	c.windowMu.Unlock()
	if c.ctx.Err() != nil || parent == nil || parent.IsDestroyed() {
		return nil
	}
	if c.permissionWindow != nil {
		c.closePermissionPrompts()
		c.permissionWindow.Close()
	}
	ctx, cancel := context.WithCancel(c.ctx)
	p := &packageUpdatePrompt{client: c, ctx: ctx, cancel: cancel, required: required, minimum: minimum,
		title: "检查更新", detail: "检查客户端程序和前端资源…", label: "检查更新"}
	if required {
		p.title = "需要更新客户端"
	}
	p.window = mygo.NewWindow(mygo.WindowOptions{
		Parent: parent, Modal: true, Title: "客户端更新", Width: 460, Height: 320,
		UseContentSize: true, Frameless: true, CornerRadius: 20, SkipTaskbar: true,
		DisableResize: true, DisableMinimize: true, DisableMaximize: true, DisableFullScreen: true,
		BackgroundColor: "light-dark(#ffffff,#252733)", Content: ui.View(p.view),
	})
	c.packagePrompt = p
	p.window.OnClose(func(e *mygo.CloseEvent) {
		if p.exiting {
			return
		}
		if p.busy {
			e.PreventDefault()
			return
		}
		if p.required {
			e.PreventDefault()
			p.dismiss()
		}
	})
	p.window.OnClosed(func() {
		cancel()
		if c.packagePrompt == p {
			c.packagePrompt = nil
		}
		c.packageChecking.Store(false)
	})
	return p
}

func (p *packageUpdatePrompt) change(fn func()) {
	mygo.RunOnMain(func() {
		if p.ctx.Err() != nil || p.window.IsDestroyed() {
			return
		}
		fn()
		p.window.Invalidate()
	})
}

func (p *packageUpdatePrompt) check() {
	if p.busy {
		return
	}
	p.busy, p.closeOnAccept, p.update, p.safetyFailed = true, false, nil, false
	p.detail = "正在检查客户端程序和前端资源…"
	p.window.Invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(p.ctx, 90*time.Second)
		defer cancel()
		var update *mygo.Update
		var packageError, frontendError error
		var revision string
		var group sync.WaitGroup
		group.Add(2)
		go func() { defer group.Done(); update, packageError = mygo.Updater.Check(ctx) }()
		go func() {
			defer group.Done()
			if p.client.updates == nil {
				frontendError = errors.New("frontend not initialized")
				return
			}
			revision, frontendError = p.client.updates.Check(ctx)
			if frontendError == nil && revision != "" {
				var refreshAvailable bool
				refreshAvailable, frontendError = p.client.stageFrontend(revision)
				if !refreshAvailable && frontendError == nil {
					revision = ""
				}
			}
		}()
		group.Wait()
		var required *webfrontend.UpdateRequiredError
		if errors.As(frontendError, &required) {
			p.client.packageRequired.Store(true)
		}
		p.change(func() {
			p.busy, p.update = false, update
			if required != nil {
				p.required, p.minimum = true, required.MinimumVersion
			}
			if p.required {
				p.title = "需要更新客户端"
				p.label = "重试检查"
				if packageError != nil {
					p.detail = "服务器要求升级客户端后才能继续使用。更新检查失败，请检查网络后重试。"
				} else if update == nil || !packageVersionAtLeast(update.Version, p.minimum) {
					p.update = nil
					p.detail = "服务器要求升级客户端后才能继续使用，兼容安装包暂未就绪，请稍后重试。"
				} else {
					p.detail = "服务器要求更新至兼容版本，正在保存草稿并准备更新。安装后将自动重启，账号与设置会保留。"
					p.label = "保存并更新"
					p.accept()
				}
				return
			}
			p.title, p.label = "检查更新", "重新检查"
			if update != nil {
				p.title, p.label = "发现新版本 "+update.Version, "下载并安装"
				p.detail = "客户端宿主与内置关于页有可用更新。安装后重启生效，账号和设置会保留。"
				if frontendError != nil {
					p.detail += " 前端资源检查暂时失败，可稍后重试。"
				}
			} else if packageError != nil || frontendError != nil {
				p.detail = "检查未完成，请检查网络后重试。"
				if errors.Is(packageError, mygo.ErrUpdatesDisabled) {
					p.detail = "当前开发版本或安装目录不支持程序自动更新。"
				}
				if revision != "" && frontendError == nil {
					p.detail += " 前端更新已准备好，将在下次启动或刷新时生效。"
				}
			} else if revision != "" {
				p.detail, p.label, p.closeOnAccept = "前端更新已准备好，将在下次启动或点击页面的“刷新并更新”时生效。客户端程序已是最新版本。", "知道了", true
			} else {
				p.detail, p.label, p.closeOnAccept = "客户端程序和前端资源均已是最新版本。", "确定", true
			}
		})
	}()
}

func (p *packageUpdatePrompt) dismiss() {
	if p.busy {
		return
	}
	if p.resumeEditing() {
		p.window.Close()
		return
	}
	if !p.required {
		p.window.Close()
		return
	}
	p.busy = true
	go func() {
		safe := p.client.preparePackageRestart(true)
		p.change(func() {
			p.busy = false
			if !safe {
				p.failedRestartPreparation()
				return
			}
			p.exiting = true
			mygo.App.Quit()
		})
	}()
}

// Returning to an existing editor dismisses only the modal. The server version
// gate remains active until the application has actually been updated.
func (p *packageUpdatePrompt) resumeEditing() bool {
	if !p.required || !p.safetyFailed || p.busy {
		return false
	}
	p.exiting = true
	return true
}

func (p *packageUpdatePrompt) failedRestartPreparation() {
	p.busy = false
	p.safetyFailed = p.required
	p.detail = "请先保存编辑内容、等待上传完成后重试。草稿保存失败时不会重启。"
	if p.required {
		p.detail = "编辑内容尚未安全保存。可返回已有页面复制或保存内容，完成后点击左上角营地标识中的“检查更新”继续。服务器版本限制仍然生效。"
	}
}

func (p *packageUpdatePrompt) accept() {
	if p.busy {
		return
	}
	if p.closeOnAccept && !p.required {
		p.window.Close()
		return
	}
	if p.update == nil && !p.installed {
		p.check()
		return
	}
	required, installed, update := p.required, p.installed, p.update
	p.busy = true
	p.window.Invalidate()
	go func() {
		if !p.client.preparePackageRestart(required) {
			p.change(func() {
				p.failedRestartPreparation()
			})
			return
		}
		if installed {
			p.restart()
			return
		}
		p.change(func() { p.detail = "正在下载并验证更新签名…" })
		ctx, cancel := context.WithTimeout(p.ctx, 10*time.Minute)
		percent := -1
		err := update.Install(ctx, func(done, total int64) {
			if total <= 0 {
				return
			}
			next := int(done * 100 / total)
			if next == percent {
				return
			}
			percent = next
			text := fmt.Sprintf("正在下载并验证更新：%d%%", percent)
			p.change(func() { p.detail = text })
		})
		cancel()
		p.change(func() {
			p.busy = false
			if err != nil {
				p.detail, p.label = "下载或安装失败。请检查网络与安装目录权限后重试。", "重试更新"
				return
			}
			p.installed = true
			if p.required {
				p.busy = true
				p.detail = "更新已安装，正在保存草稿并重新启动…"
				go func() {
					if !p.client.preparePackageRestart(true) {
						p.change(func() {
							p.failedRestartPreparation()
							p.label = "保存并重启"
						})
						return
					}
					p.restart()
				}()
			} else {
				p.detail, p.label = "更新已安装，重启后生效。也可以稍后手动重新打开客户端。", "重启客户端"
			}
		})
	}()
}

func (p *packageUpdatePrompt) restart() {
	p.change(func() { p.busy = false; p.exiting = true; mygo.App.Relaunch() })
}

func (p *packageUpdatePrompt) view(ctx *ui.Context) {
	t := permissionTheme(ctx.Theme().Dark)
	ctx.OnWindowShortcut(0, ui.KeyEscape, p.dismiss)
	ui.Themed(ctx, t).Fill().Padding(windowCaptionHeight+4, 28, 28, 28).Gap(18).Background(t.Background).Role(ui.RoleDialog).Label("客户端更新").Children(func() {
		ui.Text(ctx, p.title).FontSize(21)
		ui.Text(ctx, p.detail).FillWidth().FontSize(14)
		if p.required && p.minimum != "" {
			ui.Text(ctx, "最低客户端版本："+p.minimum).FontSize(12).TextColor(t.TextMuted)
		}
		ui.Spacer(ctx)
		ui.Row(ctx).FillWidth().Gap(12).Children(func() {
			dismiss := "稍后"
			if p.required {
				dismiss = "退出客户端"
				if p.safetyFailed {
					dismiss = "返回保存编辑内容"
				}
			}
			ui.Button(ctx.Key("later"), dismiss).Grow(1).Height(44).Disabled(p.busy).OnClick(p.dismiss)
			ui.PrimaryButton(ctx.Key("install"), p.label).Grow(1).Height(44).Disabled(p.busy).OnClick(p.accept)
		})
		ui.Box(ctx).Absolute().Left(0).Right(42).Top(0).Height(windowCaptionHeight).DragWindow()
		nativeCaptionControl(ctx, t, "close", "关闭更新提示", permissionCloseIcon, p.dismiss).Absolute().Right(0).Top(0)
	})
}
