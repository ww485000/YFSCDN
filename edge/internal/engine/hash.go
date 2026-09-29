package engine

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
)

func md5sum(b []byte) [16]byte  { return md5.Sum(b) }
func sha1sum(b []byte) [20]byte { return sha1.Sum(b) }
func sha256sum(b []byte) [32]byte {
	return sha256.Sum256(b)
}
