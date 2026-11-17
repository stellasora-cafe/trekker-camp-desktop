// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webfrontend

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

var ErrMaintenance = errors.New("server is under maintenance")

func IsMaintenanceURL(origin, raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Path != "/__camp-maintenance/v1.html" {
		return false
	}
	base, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return u.Scheme == base.Scheme && u.Host == base.Host || u.Scheme == "https" && u.Host == "camp.stellasora.cafe"
}

// ProbeOrigin bypasses the WebView's local document interception.
func ProbeOrigin(ctx context.Context, origin string) (bool, error) {
	return ProbeTarget(ctx, origin, origin+"/")
}

// ProbeTarget checks the actual entrypoint that redirected into maintenance.
func ProbeTarget(ctx context.Context, origin, target string) (bool, error) {
	base, err := url.Parse(origin)
	if err != nil {
		return false, err
	}
	parsed, err := url.Parse(target)
	if err != nil {
		return false, err
	}
	if parsed.User != nil || parsed.Scheme != base.Scheme || parsed.Host != base.Host {
		return false, errors.New("unexpected maintenance probe origin")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Accept", "text/html")
	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if IsMaintenanceURL(origin, req.URL.String()) {
			return ErrMaintenance
		}
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		if req.URL.Scheme != via[0].URL.Scheme || req.URL.Host != via[0].URL.Host {
			return errors.New("unexpected server redirect")
		}
		return nil
	}}
	response, err := client.Do(req)
	if errors.Is(err, ErrMaintenance) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	if response.Header.Get("X-Camp-Page") == "maintenance" {
		return true, nil
	}
	if response.StatusCode >= 500 {
		return false, fmt.Errorf("maintenance probe HTTP %d", response.StatusCode)
	}
	return IsMaintenanceURL(origin, response.Request.URL.String()), nil
}
