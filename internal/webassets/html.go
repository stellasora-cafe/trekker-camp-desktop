// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webassets

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

const styleID = "camp-local-assets"

var cssURL = regexp.MustCompile(`url\(\s*["']?([^"')]+)["']?\s*\)`)

// Style returns CSS that can also be inserted after document-start navigation.
// Separate family names keep later Vite styles from replacing cached faces.
// Missing cached faces use their source URLs with the desktop weight ranges.
func (b *Bundle) Style() string {
	var css strings.Builder
	for _, face := range faces {
		name := "MiSans-" + face.name + ".woff2"
		src := b.origin + "/client-assets/camp/fonts/misans-v1/" + name
		if _, ok := b.resources["/private/fonts/"+name]; ok {
			src = AssetBaseURL + "private/fonts/" + name
		}
		fmt.Fprintf(&css, "@font-face{font-family:'Camp Cached MiSans';src:url('%s') format('woff2');font-weight:%s;font-style:normal;font-display:swap}\n", src, face.weight)
	}
	css.WriteString(":root{--MI-font:'Camp Cached MiSans','MiSans','Hiragino Maru Gothic Pro','BIZ UDGothic',Roboto,HelveticaNeue,Arial,sans-serif,'Apple Color Emoji','Segoe UI Emoji','Segoe UI Symbol','Noto Color Emoji' !important}\n")
	if r, ok := b.resources["/public/icons/tabler-icons-frontend.css"]; ok {
		if data, err := os.ReadFile(b.filename("public/icons/tabler-icons-frontend.css")); err == nil {
			css.WriteString(b.iconCSS(string(data), r.Remote))
			css.WriteString("\n:root .ti{font-family:'Camp Cached Tabler','tabler-icons' !important}\n")
		}
	}
	return css.String()
}

func (b *Bundle) iconCSS(css, remote string) string {
	base, _ := url.Parse(b.origin + remote)
	css = cssURL.ReplaceAllStringFunc(css, func(match string) string {
		parts := cssURL.FindStringSubmatch(match)
		reference, err := url.Parse(strings.TrimSpace(parts[1]))
		if err != nil {
			return match
		}
		resolved := base.ResolveReference(reference)
		if resolved.Scheme != base.Scheme || resolved.Host != base.Host {
			return match
		}
		local := b.rewrites[resolved.RequestURI()]
		if local == "" {
			// Vite development CSS expands workspace symlinks to /@fs/ URLs.
			// The only permitted font basenames remain the two explicit entries.
			name := path.Base(resolved.Path)
			if name == "tabler-icons.woff2" || name == "tabler-icons-frontend.woff2" {
				if _, ok := b.resources["/public/icons/"+name]; ok {
					local = AssetBaseURL + "public/icons/" + name
				}
			}
		}
		if local == "" {
			local = resolved.String()
		}
		return "url('" + local + "')"
	})
	css = strings.ReplaceAll(css, `"tabler-icons"`, `"Camp Cached Tabler"`)
	return strings.ReplaceAll(css, `'tabler-icons'`, `'Camp Cached Tabler'`)
}

// HTML preserves scripts, worker URLs, application origin and page content. It
// replaces only allowlisted resource attributes and appends the cached faces.
func (b *Bundle) HTML(document string) (string, error) {
	var output strings.Builder
	tokenizer := html.NewTokenizer(strings.NewReader(document))
	hasStyle := strings.Contains(document, `id="`+styleID+`"`) || strings.Contains(document, `id='`+styleID+`'`)
	headClosed := false
	for {
		typeOf := tokenizer.Next()
		if typeOf == html.ErrorToken {
			if err := tokenizer.Err(); err != io.EOF {
				return "", err
			}
			break
		}
		raw := string(tokenizer.Raw())
		if typeOf == html.EndTagToken {
			name, _ := tokenizer.TagName()
			if string(name) == "head" {
				headClosed = true
				if !hasStyle {
					if css := b.Style(); css != "" {
						output.WriteString(`<style id="` + styleID + `">`)
						output.WriteString(strings.ReplaceAll(css, "</style", `<\/style`))
						output.WriteString("</style>")
					}
				}
			}
		}
		if typeOf == html.StartTagToken || typeOf == html.SelfClosingTagToken {
			token := tokenizer.Token()
			changed := false
			for i := range token.Attr {
				attr := &token.Attr[i]
				if attr.Key != "src" && attr.Key != "href" {
					continue
				}
				if replacement := b.rewriteURL(attr.Val); replacement != "" {
					attr.Val = replacement
					changed = true
				}
			}
			if changed {
				raw = token.String()
			}
		}
		output.WriteString(raw)
	}
	if !headClosed {
		return "", fmt.Errorf("webassets: HTML has no closing head")
	}
	return output.String(), nil
}

func (b *Bundle) rewriteURL(raw string) string {
	reference, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	base, _ := url.Parse(b.origin + "/")
	resolved := base.ResolveReference(reference)
	if resolved.Scheme != base.Scheme || resolved.Host != base.Host {
		return ""
	}
	return b.rewrites[resolved.RequestURI()]
}
