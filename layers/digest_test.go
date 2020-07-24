package layers

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestConcurrentHasher(t *testing.T) {
	msg := []byte("Some important message")

	hasher := sha256.New()
	hasher.Write(msg)
	digest := hex.EncodeToString(hasher.Sum(nil))

	cHasher := newConcurrentHasher(sha256.New())
	cHasher.Write(msg)
	cDigest := hex.EncodeToString(cHasher.Sum(nil))

	if digest != cDigest {
		t.Fatalf(`both digests should be the same, got %s and %s`, digest, cDigest)
	}
}
