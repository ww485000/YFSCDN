package engine

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"hash/fnv"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"edgecdn/edge/internal/contract"
)

// compiledRewrite is one rewrite rule with its conditions pre-compiled.
type compiledRewrite struct {
	cfg   contract.Rewrite
	conds []cond
}

type cond struct {
	cfg contract.Cond
	re  *regexp.Regexp
}

func compileRewrite(rw *contract.Rewrite) (*compiledRewrite, error) {
	cr := &compiledRewrite{cfg: *rw}
	for i := range rw.Conditions {
		c := cond{cfg: rw.Conditions[i]}
		if rw.Conditions[i].Operator == "regex" && rw.Conditions[i].Value != "" {
			if re, err := regexp.Compile(rw.Conditions[i].Value); err == nil {
				c.re = re
			}
		}
		cr.conds = append(cr.conds, c)
	}
	return cr, nil
}

// eval runs the rule against the request.
// return-type: (value) is the redirect target / rewritten path; return is terminal.
func (cr *compiledRewrite) eval(r *http.Request, args *rewriteArgs) (kind rewriteKind, value string) {
	caps, ok := cr.condsAll(r)
	if !ok {
		return kindNone, ""
	}
	if cr.cfg.Type == "return" {
		return kindReturn, applyFilters(cr.cfg.Value, args, caps)
	}
	// rewrite
	out := cr.cfg.Value
	out = fillCaptures(out, caps)
	return kindRewrite, applyFilters(out, args, nil)
}

// returnStatus resolves the response code for a "return" rule.
func (cr *compiledRewrite) returnStatus() int {
	if cr.cfg.Status >= 200 && cr.cfg.Status < 600 {
		return cr.cfg.Status
	}
	return http.StatusFound
}

type rewriteKind int

const (
	kindNone rewriteKind = iota
	kindReturn
	kindRewrite
)

// rewriteArgs carries request-derived variables ($arg_x etc.).
type rewriteArgs struct {
	query url.Values
	path  string
	host  string
}

// condsAll checks every condition; returns capture groups from the first
// regex condition that matched (nil when none).
func (cr *compiledRewrite) condsAll(r *http.Request) ([]string, bool) {
	var caps []string
	for i := range cr.conds {
		c := &cr.conds[i]
		hay := condHay(c.cfg.Target, r)
		if !condMatch(c.cfg.Operator, c.cfg.Value, hay, c.re) {
			return nil, false
		}
		if c.cfg.Operator == "regex" && c.re != nil {
			if m := c.re.FindStringSubmatch(hay); m != nil && caps == nil {
				caps = m
			}
		}
	}
	return caps, true
}

// condMatch applies one operator; regex uses the pre-compiled re.
func condMatch(op, value, hay string, re *regexp.Regexp) bool {
	switch op {
	case "regex":
		return re != nil && re.MatchString(hay)
	case "equals":
		return strings.EqualFold(hay, value)
	case "not_equals":
		return !strings.EqualFold(hay, value)
	case "starts_with", "prefix":
		return strings.HasPrefix(hay, value)
	case "ends_with", "suffix":
		return strings.HasSuffix(hay, value)
	case "not_contains":
		return !strings.Contains(strings.ToLower(hay), strings.ToLower(value))
	case "contains", "":
		return strings.Contains(strings.ToLower(hay), strings.ToLower(value))
	}
	return strings.Contains(hay, value)
}

// fillCaptures substitutes $1..$9 with regex capture groups.
func fillCaptures(s string, caps []string) string {
	if len(caps) == 0 || !strings.Contains(s, "$") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '$' && i+1 < len(s) && s[i+1] >= '1' && s[i+1] <= '9' {
			n := int(s[i+1] - '0')
			if n < len(caps) {
				b.WriteString(caps[n])
			}
			i++
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func condHay(target string, r *http.Request) string {
	t := strings.ToLower(target)
	// prefix targets: header:<Name>, cookie:<Name>
	if i := strings.IndexByte(t, ':'); i > 0 {
		switch t[:i] {
		case "header":
			return r.Header.Get(target[i+1:])
		case "cookie":
			if c, err := r.Cookie(target[i+1:]); err == nil {
				return c.Value
			}
			return ""
		}
	}
	switch t {
	case "path", "url":
		return r.URL.Path
	case "uri":
		return r.RequestURI
	case "query", "arg":
		return r.URL.RawQuery
	case "host":
		return r.Host
	case "method":
		return r.Method
	case "referer":
		return r.Referer()
	case "user-agent", "user_agent", "ua":
		return r.UserAgent()
	case "ip":
		return clientIP(r)
	}
	return r.URL.Path
}

// Filters (GoEdge filterconfigs, 100% set).
// Applied to a rewrite value: tokens separated by '|' left-to-right,
// each token is a filter name, optionally with args in parens.

var filterFuncs = map[string]func(string, []string) string{}

func init() {
	filterFuncs["base64_decode"] = func(s string, _ []string) string {
		b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
		if err != nil {
			return s
		}
		return string(b)
	}
	filterFuncs["base64_encode"] = func(s string, _ []string) string {
		return base64.StdEncoding.EncodeToString([]byte(s))
	}
	filterFuncs["md5"] = func(s string, _ []string) string {
		return fmt.Sprintf("%x", md5sum([]byte(s)))
	}
	filterFuncs["sha1"] = func(s string, _ []string) string {
		return fmt.Sprintf("%x", sha1sum([]byte(s)))
	}
	filterFuncs["sha256"] = func(s string, _ []string) string {
		return fmt.Sprintf("%x", sha256sum([]byte(s)))
	}
	filterFuncs["hex2dec"] = func(s string, _ []string) string {
		n, err := strconv.ParseInt(strings.TrimSpace(s), 16, 64)
		if err != nil {
			return s
		}
		return strconv.FormatInt(n, 10)
	}
	filterFuncs["dec2hex"] = func(s string, _ []string) string {
		n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		if err != nil {
			return s
		}
		return strconv.FormatInt(n, 16)
	}
	filterFuncs["url_encode"] = func(s string, _ []string) string {
		return url.QueryEscape(s)
	}
	filterFuncs["url_decode"] = func(s string, _ []string) string {
		v, err := url.QueryUnescape(s)
		if err != nil {
			return s
		}
		return v
	}
	filterFuncs["html_escape"] = func(s string, _ []string) string {
		return html.EscapeString(s)
	}
	filterFuncs["html_unescape"] = func(s string, _ []string) string {
		return html.UnescapeString(s)
	}
	filterFuncs["xml_escape"] = func(s string, _ []string) string {
		var b strings.Builder
		xmlEscape(&b, s)
		return b.String()
	}
	filterFuncs["xml_unescape"] = func(s string, _ []string) string {
		return html.UnescapeString(s)
	}
	filterFuncs["unicode_encode"] = func(s string, _ []string) string {
		var b strings.Builder
		for _, r := range s {
			b.WriteString(fmt.Sprintf("\\u%04x", r))
		}
		return b.String()
	}
	filterFuncs["unicode_decode"] = func(s string, _ []string) string {
		return unicodeDecode(s)
	}
	filterFuncs["length"] = func(s string, _ []string) string {
		return strconv.Itoa(utf8.RuneCountInString(s))
	}
	filterFuncs["crc32"] = func(s string, _ []string) string {
		return strconv.FormatUint(uint64(crc32.ChecksumIEEE([]byte(s))), 10)
	}
	filterFuncs["fnv"] = func(s string, _ []string) string {
		h := fnv.New32a()
		io.WriteString(h, s)
		return strconv.FormatUint(uint64(h.Sum32()), 10)
	}
	filterFuncs["dec2hex_str"] = func(s string, _ []string) string {
		b := []byte(s)
		var out []byte
		for _, c := range b {
			out = append(out, hex.EncodeToString([]byte{c})...)
		}
		return string(out)
	}
}

func xmlEscape(b *strings.Builder, s string) {
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&apos;")
		default:
			b.WriteRune(r)
		}
	}
}

func unicodeDecode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+5 < len(s)+1 && i+5 <= len(s) {
			if s[i+1] == 'u' && i+5 < len(s) {
				if code, err := strconv.ParseUint(s[i+2:i+6], 16, 32); err == nil {
					b.WriteRune(rune(code))
					i += 5
					continue
				}
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// applyFilters runs a value through '|' separated filter tokens, then expands
// $host / $path / $arg_name variables.
func applyFilters(value string, args *rewriteArgs, caps []string) string {
	s := value
	if len(caps) > 0 {
		s = fillCaptures(s, caps)
	}
	parts := strings.Split(s, "|")
	if len(parts) == 1 {
		return expandVars(s, args)
	}
	head := strings.TrimSpace(parts[0])
	filtered := expandVars(head, args)
	for _, tok := range parts[1:] {
		tok = strings.TrimSpace(tok)
		name := tok
		var a []string
		if i := strings.IndexByte(tok, '('); i >= 0 && strings.HasSuffix(tok, ")") {
			name = strings.TrimSpace(tok[:i])
			a = strings.Split(tok[i+1:len(tok)-1], ",")
			for j := range a {
				a[j] = strings.TrimSpace(a[j])
			}
		}
		if fn, ok := filterFuncs[name]; ok {
			filtered = fn(filtered, a)
		}
	}
	return filtered
}

// expandVars substitutes $host / $path / $arg_name / $1..$9 (if captures exist).
func expandVars(s string, args *rewriteArgs) string {
	if !strings.Contains(s, "$") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '$' {
			b.WriteByte(s[i])
			continue
		}
		i++
		if i >= len(s) {
			b.WriteByte('$')
			break
		}
		switch {
		case s[i] >= '1' && s[i] <= '9':
			b.WriteByte(s[i]) // capture groups filled by caller via regex replace
		case s[i] == 'h' && hasPrefix(s[i:], "host"):
			i += len("host") - 1
			b.WriteString(args.host)
		case s[i] == 'p' && hasPrefix(s[i:], "path"):
			i += len("path") - 1
			b.WriteString(args.path)
		case s[i] == 'a' && hasPrefix(s[i:], "arg_"):
			j := i
			for j < len(s) && (s[j] == '_' || isAlnum(s[j])) && j-i < 64 {
				j++
			}
			name := s[i:j]
			i = j - 1
			b.WriteString(args.query.Get(strings.TrimPrefix(name, "arg_")))
		default:
			b.WriteByte('$')
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func hasPrefix(s, p string) bool { return len(s) >= len(p) && s[:len(p)] == p }

func isAlnum(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// JSON helper (used by some filters / tests).
func jsonCompact(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// ipToHex converts an IP to a hex string (filter arg).
func ipToHex(ip string) string {
	p := net.ParseIP(ip)
	if p == nil {
		return ip
	}
	return hex.EncodeToString(p.To16())
}
