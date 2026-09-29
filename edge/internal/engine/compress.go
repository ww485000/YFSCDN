package engine

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"

	"edgecdn/edge/internal/contract"

	brotli "github.com/andybalholm/brotli"
)

// compressor configures response compression for a location.
// Supported encodings: gzip (stdlib) and brotli (andybalholm/brotli).
// "deflate" in the config is accepted but served as gzip (clients that ask
// for deflate almost always accept gzip as well).
type compressor struct {
	types   map[string]bool // enabled encodings: gzip, brotli
	minSize int
}

func newCompressor(c *contract.Compression) *compressor {
	cm := &compressor{types: map[string]bool{}, minSize: c.MinSize}
	if c.MinSize <= 0 {
		cm.minSize = 256
	}
	for _, t := range c.Types {
		switch strings.ToLower(t) {
		case "gzip", "brotli":
			cm.types[strings.ToLower(t)] = true
		case "deflate":
			cm.types["gzip"] = true // degrade to gzip
		}
	}
	if len(cm.types) == 0 {
		cm.types["gzip"] = true
	}
	return cm
}

// minBytes is the minimum body size to bother compressing.
func (cm *compressor) minBytes() int { return cm.minSize }

// decide picks the encoding to use from Accept-Encoding.
func (cm *compressor) decide(accept string) string {
	for _, part := range strings.Split(accept, ",") {
		p := strings.TrimSpace(part)
		if i := strings.IndexByte(p, ';'); i >= 0 {
			p = strings.TrimSpace(p[:i])
		}
		switch strings.ToLower(p) {
		case "br":
			if cm.types["brotli"] {
				return "br"
			}
		case "gzip", "deflate":
			if cm.types["gzip"] {
				return "gzip"
			}
		case "*":
			if cm.types["brotli"] {
				return "br"
			}
			if cm.types["gzip"] {
				return "gzip"
			}
		}
	}
	return ""
}

// canCompress reports whether a response of this type is worth compressing.
func (cm *compressor) canCompress(contentType string) bool {
	ct := strings.ToLower(contentType)
	if strings.HasPrefix(ct, "image/") || strings.HasPrefix(ct, "video/") || strings.HasPrefix(ct, "audio/") {
		return false
	}
	if strings.Contains(ct, "charset=") {
		return true
	}
	switch {
	case strings.Contains(ct, "text/"),
		strings.Contains(ct, "json"),
		strings.Contains(ct, "javascript"),
		strings.Contains(ct, "xml"),
		strings.Contains(ct, "html"),
		strings.Contains(ct, "csv"),
		strings.Contains(ct, "yaml"):
		return true
	}
	return false
}

// startWriter begins streaming compression with the chosen encoding.
func (cm *compressor) startWriter(w http.ResponseWriter, r *http.Request) (io.Writer, bool) {
	enc := cm.decide(r.Header.Get("Accept-Encoding"))
	if enc == "" {
		return w, false
	}
	ct := w.Header().Get("Content-Type")
	if !cm.canCompress(ct) {
		return w, false
	}
	h := w.Header()
	h.Del("Content-Length")
	h.Add("Vary", "Accept-Encoding")
	switch enc {
	case "br":
		h.Set("Content-Encoding", "br")
		return brotli.NewWriter(w), true
	default:
		h.Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		gz.Name = ""
		return gzipCloser{gz: gz}, true
	}
}

type gzipCloser struct{ gz *gzip.Writer }

func (g gzipCloser) Write(b []byte) (int, error) { return g.gz.Write(b) }
func (g gzipCloser) Close() error               { return g.gz.Close() }

// flushCompressor closes the compressor (after body done).
func flushCompressor(w io.Writer) {
	if c, ok := w.(io.Closer); ok {
		_ = c.Close()
	}
}
