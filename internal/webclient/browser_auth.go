// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"net/url"
	"strings"
)

// Browser authorization opens only the site's consent page from the desktop
// sign-in window. Claim secrets stay in that window's memory, never in a URL.
func browserAuthorizationTarget(origin, source, target string) bool {
	if !sameOrigin(origin, source) || !sameOrigin(origin, target) {
		return false
	}
	from, err := url.Parse(source)
	if err != nil || from.Path != "/desktop-auth" {
		return false
	}
	to, err := url.Parse(target)
	if err != nil || to.RawQuery != "" || to.Fragment != "" || to.RawPath != "" || !strings.HasPrefix(to.Path, "/client-auth/") {
		return false
	}
	requestID := strings.TrimPrefix(to.Path, "/client-auth/")
	return requestID != "" && strings.Trim(requestID, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_") == ""
}
