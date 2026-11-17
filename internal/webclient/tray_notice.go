// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/egoist/mygo"
)

const trayNoticePath = "/__camp-desktop/tray-notice"
const trayNoticePrefix = "camp-tray:"

func (c *client) serveTrayNotice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !sameOrigin(c.origin, r.URL.String()) ||
		r.Header.Get("X-Camp-Tray-Notice") != "1" ||
		(r.Header.Get("Origin") != "" && r.Header.Get("Origin") != c.origin) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if !c.inTray.Load() {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Body == nil {
		http.Error(w, "invalid notification", http.StatusBadRequest)
		return
	}

	const limit = 4096
	var notice struct {
		ID, Title, Body string
		Silent          bool
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil || len(body) > limit || json.Unmarshal(body, &notice) != nil ||
		notice.ID == "" || notice.Title == "" || notice.Body == "" {
		http.Error(w, "invalid notification", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
	go mygo.RunOnMain(func() {
		if c.inTray.Load() && !c.quitting && c.ctx.Err() == nil {
			_ = mygo.NewNotification(mygo.NotificationOptions{
				ID: trayNoticePrefix + notice.ID, Title: notice.Title, Body: notice.Body, Silent: notice.Silent,
			}).ShowContext(c.ctx)
		}
	})
}
