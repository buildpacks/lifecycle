package layers

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestConcurrentHasher(t *testing.T) {
	msg := []byte("Some important message")

	hasher := sha256.New()
	_, err := hasher.Write(msg)
	h.AssertNil(t, err)
	digest := hex.EncodeToString(hasher.Sum(nil))

	cHasher := newConcurrentHasher(sha256.New())
	_, err = cHasher.Write(msg)
	h.AssertNil(t, err)
	cDigest := hex.EncodeToString(cHasher.Sum(nil))

	if digest != cDigest {
		t.Fatalf(`both digests should be the same, got %s and %s`, digest, cDigest)
	}
}
