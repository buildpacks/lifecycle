package buildpack_test

import (
	"errors"
	"testing"

	"github.com/buildpacks/lifecycle/buildpack"
)

func TestError(t *testing.T) {
	t.Run("#Cause", func(t *testing.T) {
		t.Run("returns the error", func(t *testing.T) {
			expectedErr := errors.New("root cause")
			testErr := &buildpack.Error{
				RootError: expectedErr,
			}

			cause := testErr.Cause()

			if cause != expectedErr {
				t.Fatalf("Unexpected cause:\n%s\n", cause)
			}
		})
		t.Run("returns handles nil state", func(t *testing.T) {
			testErr := &buildpack.Error{}

			if testErr.Cause() != nil {
				t.Fatalf("Unexpected cause:\n%s\n", testErr.Cause())
			}
		})
	})
	t.Run("#Error", func(t *testing.T) {
		t.Run("returns the underlying error", func(t *testing.T) {
			expectedErr := errors.New("root cause")
			testErr := &buildpack.Error{
				RootError: expectedErr,
			}

			if testErr.Error() != expectedErr.Error() {
				t.Fatalf("Unexpected error:\n%s\n", testErr.Error())
			}
		})
		t.Run("returns the type when there is no error", func(t *testing.T) {
			testErr := &buildpack.Error{
				Type: buildpack.ErrTypeBuildpack,
			}

			if testErr.Error() != "ERR_BUILDPACK" {
				t.Fatalf("Unexpected error value:\n%s\n", testErr.Error())
			}
		})
	})
}
