package dnsprovider

import (
	"testing"

	"edgecdn/core/internal/dns"
)

func TestDNSPodSubDomain(t *testing.T) {
	cases := []struct {
		name string
		rec  dns.Record
		want string
	}{
		{name: "apex by domain", rec: dns.Record{Domain: "example.com", Name: "example.com"}, want: "@"},
		{name: "apex by blank", rec: dns.Record{Domain: "example.com", Name: ""}, want: "@"},
		{name: "simple host", rec: dns.Record{Domain: "example.com", Name: "www.example.com"}, want: "www"},
		{name: "nested host", rec: dns.Record{Domain: "example.com", Name: "api.dev.example.com"}, want: "api.dev"},
		{name: "relative host", rec: dns.Record{Domain: "example.com", Name: "cdn"}, want: "cdn"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := dnspodSubDomain(tc.rec); got != tc.want {
				t.Fatalf("dnspodSubDomain() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDNSPodLine(t *testing.T) {
	if got := dnspodLine(""); got != "默认" {
		t.Fatalf("dnspodLine blank = %q", got)
	}
	if got := dnspodLine("telecom"); got != "telecom" {
		t.Fatalf("dnspodLine custom = %q", got)
	}
}

func TestAliyunRR(t *testing.T) {
	cases := []struct {
		name string
		rec  dns.Record
		want string
	}{
		{name: "apex by domain", rec: dns.Record{Domain: "example.com", Name: "example.com"}, want: "@"},
		{name: "apex by blank", rec: dns.Record{Domain: "example.com", Name: ""}, want: "@"},
		{name: "simple host", rec: dns.Record{Domain: "example.com", Name: "www.example.com"}, want: "www"},
		{name: "nested host", rec: dns.Record{Domain: "example.com", Name: "api.dev.example.com"}, want: "api.dev"},
		{name: "relative host", rec: dns.Record{Domain: "example.com", Name: "cdn"}, want: "cdn"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := aliyunRR(tc.rec); got != tc.want {
				t.Fatalf("aliyunRR() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAliyunLine(t *testing.T) {
	if got := aliyunLine("default"); got != "" {
		t.Fatalf("aliyunLine default = %q", got)
	}
	if got := aliyunLine("telecom"); got != "telecom" {
		t.Fatalf("aliyunLine custom = %q", got)
	}
}

func TestBase64Encode(t *testing.T) {
	if got := base64Encode([]byte("hello")); got != "aGVsbG8=" {
		t.Fatalf("base64Encode() = %q", got)
	}
}
