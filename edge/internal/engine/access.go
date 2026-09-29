package engine

import (
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"

	"edgecdn/edge/internal/contract"
)

// accessCtl compiles the location access rules (IP / Referer / UA).
// Allow lists are exclusive: when non-empty, only listed values pass.
type accessCtl struct {
	ipDeny   []*net.IPNet
	ipAllow  []*net.IPNet
	refDeny  []*refRule
	refAllow []*refRule
	uaDeny   []*refRule
	uaAllow  []*refRule
}

type refRule struct {
	re *regexp.Regexp
}

func newRefRule(v string) *refRule {
	// Values containing regex metacharacters are treated as regex (GoEdge style,
	// e.g. "^BadBot" denies user agents starting with BadBot).
	if strings.ContainsAny(v, "^\\[.*?") {
		if re, err := regexp.Compile(v); err == nil {
			return &refRule{re: re}
		}
	}
	re, _ := regexp.Compile("(?i)" + regexp.QuoteMeta(v))
	return &refRule{re: re}
}

func (rr *refRule) match(s string) bool {
	return rr.re.MatchString(s)
}

func compileAccess(a *contract.Access) *accessCtl {
	if a == nil {
		return nil
	}
	ctl := &accessCtl{}
	if a.IPs != nil {
		for _, cidr := range a.IPs.Deny {
			if ipn, err := parseCIDR(cidr); err == nil {
				ctl.ipDeny = append(ctl.ipDeny, ipn)
			}
		}
		for _, cidr := range a.IPs.Allow {
			if ipn, err := parseCIDR(cidr); err == nil {
				ctl.ipAllow = append(ctl.ipAllow, ipn)
			}
		}
	}
	if a.Referers != nil {
		for _, v := range a.Referers.Deny {
			ctl.refDeny = append(ctl.refDeny, newRefRule(v))
		}
		for _, v := range a.Referers.Allow {
			ctl.refAllow = append(ctl.refAllow, newRefRule(v))
		}
	}
	if a.UserAgents != nil {
		for _, v := range a.UserAgents.Deny {
			ctl.uaDeny = append(ctl.uaDeny, newRefRule(v))
		}
		for _, v := range a.UserAgents.Allow {
			ctl.uaAllow = append(ctl.uaAllow, newRefRule(v))
		}
	}
	return ctl
}

// parseCIDR accepts a single IP or a CIDR.
func parseCIDR(s string) (*net.IPNet, error) {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, "/") {
		if ip := net.ParseIP(s); ip != nil {
			bits := 32
			if ip.To4() == nil {
				bits = 128
			}
			return &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)}, nil
		}
		return nil, fmt.Errorf("bad ip %q", s)
	}
	_, ipn, err := net.ParseCIDR(s)
	return ipn, err
}

// check returns an error string when the request must be rejected.
func (ctl *accessCtl) check(r *http.Request) string {
	if ctl == nil {
		return ""
	}
	if p := net.ParseIP(clientIP(r)); p != nil {
		if len(ctl.ipAllow) > 0 {
			for _, ipn := range ctl.ipAllow {
				if ipn.Contains(p) {
					return ""
				}
			}
			return "ip not allowed"
		}
		for _, ipn := range ctl.ipDeny {
			if ipn.Contains(p) {
				return "ip denied"
			}
		}
	}
	if ref := r.Referer(); ref != "" {
		for _, rr := range ctl.refDeny {
			if rr.match(ref) {
				return "referer denied"
			}
		}
		if len(ctl.refAllow) > 0 {
			allowed := false
			for _, rr := range ctl.refAllow {
				if rr.match(ref) {
					allowed = true
					break
				}
			}
			if !allowed {
				return "referer not allowed"
			}
		}
	} else if len(ctl.refAllow) > 0 {
		return "referer not allowed"
	}
	ua := r.UserAgent()
	for _, rr := range ctl.uaDeny {
		if rr.match(ua) {
			return "user agent denied"
		}
	}
	if len(ctl.uaAllow) > 0 {
		allowed := false
		for _, rr := range ctl.uaAllow {
			if rr.match(ua) {
				allowed = true
				break
			}
		}
		if !allowed {
			return "user agent not allowed"
		}
	}
	return ""
}
