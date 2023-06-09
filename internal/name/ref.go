package name

import "github.com/google/go-containerregistry/pkg/name"

func ParseMaybe(ref string) string {
	if nameRef, err := name.ParseReference(ref); err == nil {
		return nameRef.Context().Name()
	}
	return ref
}
