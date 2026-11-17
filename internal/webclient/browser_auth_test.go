// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import "testing"

func TestBrowserAuthorizationTarget(t *testing.T) {
	origin := "https://camp.example"
	source := origin + "/desktop-auth?mode=signin"
	target := origin + "/client-auth/82080f26-63d5-46b2-b59d-11885c64c846"
	if !browserAuthorizationTarget(origin, source, target) {
		t.Fatal("desktop sign-in must open the site's browser consent page")
	}
	if !browserAuthorizationTarget("http://localhost:3000", "http://localhost:3000/desktop-auth", "http://localhost:3000/client-auth/local-request") {
		t.Fatal("local development authorization must remain available")
	}
	for _, value := range []string{
		"https://external.example/client-auth/request", "javascript:alert(1)",
		origin + "/", origin + "/client-auth/", origin + "/client-auth/a/b",
		target + "?secret=must-not-leave-app", target + "#secret", origin + "/client-auth/request%2Fextra",
		"https://user:password@camp.example/client-auth/request",
	} {
		if browserAuthorizationTarget(origin, source, value) {
			t.Errorf("unexpected external browser target: %s", value)
		}
	}
	for _, value := range []string{origin + "/notes/example", "about:blank", "https://outside.example/desktop-auth"} {
		if browserAuthorizationTarget(origin, value, target) {
			t.Errorf("unexpected browser authorization source: %s", value)
		}
	}
}
