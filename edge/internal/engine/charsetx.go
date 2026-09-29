package engine

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"edgecdn/edge/internal/contract"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// charsetConverter converts response bodies between charsets.
type charsetConverter struct {
	source     encoding.Encoding // nil = auto-detect from Content-Type
	auto       bool
	target     encoding.Encoding
	targetName string
}

func newCharsetConverter(c *contract.Charset) (*charsetConverter, error) {
	dst, err := lookupEncoding(c.Encoding)
	if err != nil {
		return nil, err
	}
	cc := &charsetConverter{target: dst, targetName: c.Encoding}
	if cc.targetName == "" {
		cc.targetName = "utf-8"
	}
	if c.ConvertFrom != "" {
		src, err := lookupEncoding(c.ConvertFrom)
		if err != nil {
			return nil, err
		}
		cc.source = src
	} else {
		cc.auto = true
	}
	return cc, nil
}

func lookupEncoding(name string) (encoding.Encoding, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "utf-8", "utf8":
		return unicode.UTF8, nil
	case "gbk", "gb2312", "iso-8859-1", "latin1", "latin-1", "windows-1252":
		return charmap.Windows1252, nil
	case "gb18030":
		return simplifiedchinese.GB18030, nil
	case "big5":
		return traditionalchinese.Big5, nil
	case "shift-jis", "shift_jis", "sjis":
		return japanese.ShiftJIS, nil
	case "euc-jp":
		return japanese.EUCJP, nil
	case "euc-kr":
		return korean.EUCKR, nil
	case "utf-16le", "utf16le":
		return unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM), nil
	case "utf-16be", "utf16be":
		return unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM), nil
	}
	return nil, fmt.Errorf("unsupported charset %q", name)
}

// matches reports whether a content type may carry text (html/text).
func (cc *charsetConverter) matches(contentType string) bool {
	ct := strings.ToLower(contentType)
	return strings.Contains(ct, "text/html") || strings.Contains(ct, "text/plain") || strings.Contains(ct, "xml")
}

// charsetOf extracts a charset parameter from a content type, or "".
func charsetOf(contentType string) string {
	for _, part := range strings.Split(contentType, ";") {
		p := strings.TrimSpace(part)
		if strings.HasPrefix(strings.ToLower(p), "charset=") {
			return strings.ToLower(strings.TrimPrefix(strings.ToLower(p), "charset="))
		}
	}
	return ""
}

// convert re-encodes body to the target encoding.
// source is taken from cc.source, or auto-detected from the content type.
// It returns the converted bytes and the target charset name.
func (cc *charsetConverter) convert(body []byte, contentType string) ([]byte, string, error) {
	src := cc.source
	if cc.auto {
		name := charsetOf(contentType)
		if name == "" || name == cc.targetName {
			return body, cc.targetName, nil // nothing to convert
		}
		s, err := lookupEncoding(name)
		if err != nil {
			return body, cc.targetName, nil // unsupported source; pass through
		}
		src = s
	}
	if src == nil || src == cc.target {
		return body, cc.targetName, nil
	}
	// Decode source -> UTF-8.
	utf8Bytes, _, err := transform.Bytes(src.NewDecoder(), body)
	if err != nil {
		return body, cc.targetName, nil // malformed; pass through
	}
	// Encode UTF-8 -> target (no-op when target is UTF-8).
	if cc.targetName != "utf-8" && cc.targetName != "utf8" {
		utf8Bytes, _, err = transform.Bytes(cc.target.NewEncoder(), utf8Bytes)
		if err != nil {
			return body, cc.targetName, nil
		}
	}
	return utf8Bytes, cc.targetName, nil
}

func isUTF8(b []byte) bool {
	return utf8.Valid(b)
}
