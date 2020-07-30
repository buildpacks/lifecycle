package lifecycle_test

import (
	"testing"

	"errors"

	"github.com/sclevine/spec"

	"github.com/buildpacks/lifecycle"
)

func TestError(t *testing.T) {
	spec.Run(t, "Test Error", testError)
}

func testError(t *testing.T, when spec.G, it spec.S) {
	when("#Cause", func() {
		it("returns the first error", func() {
			expectedErr := errors.New("root cause")
			testErr := &lifecycle.Error{
				Errors: []error{expectedErr, errors.New("another")},
			}

			cause := testErr.Cause()

			if cause != expectedErr {
				t.Fatalf("Unexpected cause:\n%s\n", cause)
			}
		})

		it("returns handles nil state", func() {
			testErr := &lifecycle.Error{}

			if testErr.Cause() != nil {
				t.Fatalf("Unexpected cause:\n%s\n", testErr.Cause())
			}
		})
	})
}
