package buildpack_test

import (
	"errors"
	"testing"

	"github.com/sclevine/spec"

	"github.com/buildpacks/lifecycle/buildpack"
)

func TestError(t *testing.T) {
	spec.Run(t, "Test Error", testError)
}

func testError(t *testing.T, when spec.G, it spec.S) {
	when("#Cause", func() {
		it("returns the error", func() {
			expectedErr := errors.New("root cause")
			testErr := &buildpack.Error{
				RootError: expectedErr,
			}

			cause := testErr.Cause()

			if cause != expectedErr {
				t.Fatalf("Unexpected cause:\n%s\n", cause)
			}
		})

		it("returns handles nil state", func() {
			testErr := &buildpack.Error{}

			if testErr.Cause() != nil {
				t.Fatalf("Unexpected cause:\n%s\n", testErr.Cause())
			}
		})
	})

	when("#Error", func() {
		it("returns the underlying error", func() {
			expectedErr := errors.New("root cause")
			testErr := &buildpack.Error{
				RootError: expectedErr,
			}

			if testErr.Error() != expectedErr.Error() {
				t.Fatalf("Unexpected error:\n%s\n", testErr.Error())
			}
		})

		it("returns the type when there is no error", func() {
			testErr := &buildpack.Error{
				Type: buildpack.ErrTypeBuildpack,
			}

			if testErr.Error() != "ERR_BUILDPACK" {
				t.Fatalf("Unexpected error value:\n%s\n", testErr.Error())
			}
		})
	})
}
