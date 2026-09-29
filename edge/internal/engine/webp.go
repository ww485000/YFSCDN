package engine

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"strings"

	"edgecdn/edge/internal/contract"

	_ "image/jpeg"

	"github.com/HugoSmits86/nativewebp"
)

var errNotWebpCandidate = errors.New("image is not jpeg/png")

// webpConverter transcodes JPEG/PNG responses to WebP when the client accepts it.
// Pure-Go encoder (nativewebp, lossless VP8L) — no C toolchain required.
type webpConverter struct{}

func newWebpConverter(w *contract.WebP) *webpConverter {
	return &webpConverter{}
}

const maxTransformBytes = 10 << 20 // 10MB cap for buffered transforms

// acceptsWebP checks the client's Accept header.
func (wc *webpConverter) acceptsWebP(r *http.Request) bool {
	accept := strings.ToLower(r.Header.Get("Accept"))
	return strings.Contains(accept, "image/webp")
}

// matches reports whether the content type is a candidate.
func (wc *webpConverter) matches(contentType string) bool {
	ct := strings.ToLower(contentType)
	return strings.Contains(ct, "image/jpeg") || strings.Contains(ct, "image/png")
}

// convert decodes the image and re-encodes as WebP.
func (wc *webpConverter) convert(body []byte) ([]byte, string, error) {
	var img image.Image
	var err error
	if bytes.HasPrefix(body, jpegHeader) {
		img, err = jpeg.Decode(bytes.NewReader(body))
	} else if isPng(body) {
		img, err = png.Decode(bytes.NewReader(body))
	} else {
		return nil, "", errNotWebpCandidate
	}
	if err != nil {
		return nil, "", err
	}
	buf := new(bytes.Buffer)
	if err := nativewebp.Encode(buf, img, nil); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), "image/webp", nil
}

var jpegHeader = []byte{0xFF, 0xD8, 0xFF}

func isPng(b []byte) bool {
	return len(b) >= 8 && bytes.Equal(b[:8], pngHeader)
}

var pngHeader = []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
