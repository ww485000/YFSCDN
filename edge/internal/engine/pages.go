package engine

import (
	"net/http"
	"regexp"
	"strings"
	"time"

	"edgecdn/edge/internal/contract"
)

// serveError renders a custom error page (or a default one) with the given status.
func serveError(w http.ResponseWriter, r *http.Request, status int, title string, pages map[int]*contract.ErrorPage) {
	if p, ok := pages[status]; ok && p.Enabled {
		h := w.Header()
		h.Set("Content-Type", "text/html; charset=utf-8")
		body := p.Content
		body = strings.ReplaceAll(body, "$status", itoa(status))
		body = strings.ReplaceAll(body, "$title", p.Title)
		body = strings.ReplaceAll(body, "$host", r.Host)
		h.Set("Content-Length", itoa(len(body)))
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
		return
	}
	// default minimal page
	body := defaultErrorPage(status, title)
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Length", itoa(len(body)))
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func defaultErrorPage(status int, title string) string {
	if title == "" {
		title = http.StatusText(status)
	}
	return `<html><head><title>` + itoa(status) + `</title></head><body>` +
		`<h1>` + itoa(status) + `</h1><p>` + escapeHTML(title) + `</p>` +
		`<hr><p>edgecdn</p></body></html>`
}

func escapeHTML(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

// minify HTML: strip comments and (mostly) whitespace between tags.
var (
	htmlCommentRe = regexp.MustCompile(`(?s)<!--.*?-->`)
	cssCommentRe  = regexp.MustCompile(`(?s)/\*.*?\*/`)
	jsLineRe      = regexp.MustCompile(`(?m)^\s*//.*$`)
	wsBetweenRe   = regexp.MustCompile(`[\r\n\t ]+`)
)

// minifyHTML returns a shrunken copy of the HTML document.
func minifyHTML(m *contract.Minify, body []byte) []byte {
	if m == nil || !m.HTML {
		return body
	}
	s := string(body)
	s = htmlCommentRe.ReplaceAllString(s, "")
	s = cssCommentRe.ReplaceAllString(s, "")
	s = jsLineRe.ReplaceAllString(s, "")
	// collapse runs of whitespace between tags
	type cut struct{ a, b int }
	var b strings.Builder
	last := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '<' || s[i] == '>' {
			if i > last {
				b.WriteString(wsBetweenRe.ReplaceAllString(s[last:i], " "))
			}
			b.WriteByte(s[i])
			last = i + 1
		}
	}
	if last < len(s) {
		b.WriteString(wsBetweenRe.ReplaceAllString(s[last:], " "))
	}
	return []byte(b.String())
}

// expiresNow builds the Expires header value.
func expiresNow(d time.Duration) string {
	return time.Now().UTC().Add(d).Format(http.TimeFormat)
}
