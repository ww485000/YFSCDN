package engine

import (
	"net/http"
	"net/netip"
	"regexp"
	"strings"

	"edgecdn/edge/internal/contract"
)

// wafEngine evaluates WAF rules (IP lists first, then rule engine).
// GoEdge WAF semantics: rules with action=block return 403;
// action=captcha/js are challenge responses; action=log only records.
type wafEngine struct {
	ips   *ipMatcher
	rules []*compiledWAFRule
}

type compiledWAFRule struct {
	cfg    contract.WAFRule
	target string
	re     *regexp.Regexp
}

func newWafEngine(ips *contract.IPList, rules []contract.WAFRule) *wafEngine {
	w := &wafEngine{}
	if ips != nil && (len(ips.Allow) > 0 || len(ips.Deny) > 0) {
		w.ips = &ipMatcher{}
		for _, s := range ips.Allow {
			if p, err := netip.ParsePrefix(strings.TrimSpace(s)); err == nil {
				w.ips.allow = append(w.ips.allow, p)
			}
		}
		for _, s := range ips.Deny {
			if p, err := netip.ParsePrefix(strings.TrimSpace(s)); err == nil {
				w.ips.deny = append(w.ips.deny, p)
			}
		}
	}
	for i := range rules {
		r := rules[i]
		if r.Status == "off" || strings.EqualFold(r.Action, "allow") {
			continue
		}
		cr := &compiledWAFRule{cfg: r, target: strings.ToLower(r.Target)}
		if r.Operator == "regex" && r.Value != "" {
			if re, err := regexp.Compile(r.Value); err == nil {
				cr.re = re
			}
		}
		w.rules = append(w.rules, cr)
	}
	return w
}

// verdict is the WAF outcome for one request.
type verdict struct {
	action string // block | captcha | js | log
	name   string
}

// Check evaluates; nil = pass.
func (w *wafEngine) Check(r *http.Request) *verdict {
	ip := clientIP(r)
	// IP allow/deny (deny wins)
	if w.ips != nil {
		if w.ips.denyIP(ip) {
			return &verdict{action: "block", name: "ip-deny"}
		}
		if w.ips.allowIP(ip) {
			return nil
		}
	}
	for _, rule := range w.rules {
		if ruleMatch(rule, r, ip) {
			if rule.cfg.Action == "log" {
				continue
			}
			return &verdict{action: rule.cfg.Action, name: rule.cfg.Name}
		}
	}
	return nil
}

func ruleMatch(c *compiledWAFRule, r *http.Request, ip string) bool {
	var hay string
	switch {
	case strings.HasPrefix(c.target, "header:"):
		hay = r.Header.Get(c.target[len("header:"):])
	case strings.HasPrefix(c.target, "cookie:"):
		if ck, err := r.Cookie(c.target[len("cookie:"):]); err == nil {
			hay = ck.Value
		}
	case c.target == "url" || c.target == "path":
		hay = r.URL.Path
	case c.target == "query":
		hay = r.URL.RawQuery
	case c.target == "body":
		return false // body matching not implemented
	case c.target == "ip":
		hay = ip
	case c.target == "user-agent", c.target == "user_agent":
		hay = r.UserAgent()
	case c.target == "referer":
		hay = r.Referer()
	case c.target == "method":
		hay = r.Method
	case c.target == "host":
		hay = r.Host
	default:
		return false
	}
	switch c.cfg.Operator {
	case "regex":
		return c.re != nil && c.re.MatchString(hay)
	case "equals":
		return strings.EqualFold(hay, c.cfg.Value)
	case "not_equals":
		return !strings.EqualFold(hay, c.cfg.Value)
	case "starts_with":
		return strings.HasPrefix(hay, c.cfg.Value)
	case "ends_with":
		return strings.HasSuffix(hay, c.cfg.Value)
	case "not_contains":
		return !strings.Contains(strings.ToLower(hay), strings.ToLower(c.cfg.Value))
	case "contains", "":
		return strings.Contains(strings.ToLower(hay), strings.ToLower(c.cfg.Value))
	}
	return strings.Contains(hay, c.cfg.Value)
}

// ipMatcher caches parsed CIDRs.
type ipMatcher struct {
	allow []netip.Prefix
	deny  []netip.Prefix
}

func (m *ipMatcher) denyIP(ip string) bool {
	p, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	for _, d := range m.deny {
		if d.Contains(p) {
			return true
		}
	}
	return false
}

func (m *ipMatcher) allowIP(ip string) bool {
	p, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	for _, a := range m.allow {
		if a.Contains(p) {
			return true
		}
	}
	return false
}
