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

func TestBase64Encode(t *testing.T) {
	if got := base64Encode([]byte("hello")); got != "aGVsbG8=" {
		t.Fatalf("base64Encode() = %q", got)
	}
}
