package name_test

import (
	"testing"

	"github.com/buildpacks/lifecycle/internal/name"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestRef(t *testing.T) {
	t.Run(".ParseMaybe", func(t *testing.T) {
		t.Run("provided reference", func(t *testing.T) {
			type testCase struct {
				condition string
				provided  string
				does      string
				expected  string
			}
			testCases := []testCase{
				{
					condition: "is invalid",
					provided:  "!@#$",
					does:      "returns the provided reference",
					expected:  "!@#$",
				},
				{
					condition: "has an implicit registry",
					provided:  "some-library/some-repo:some-tag",
					does:      "adds an explicit registry",
					expected:  "index.docker.io/some-library/some-repo:some-tag",
				},
				{
					condition: "has an implicit library",
					provided:  "some.registry/some-repo:some-tag",
					does:      "returns the provided reference",
					expected:  "some.registry/some-repo:some-tag",
				},
				{
					condition: "has an implicit library and has registry index.docker.io",
					provided:  "index.docker.io/some-repo:some-tag",
					does:      "adds an explicit library",
					expected:  "index.docker.io/library/some-repo:some-tag",
				},
				{
					condition: "has an implicit tag",
					provided:  "some.registry/some-library/some-repo",
					does:      "adds an explicit tag",
					expected:  "some.registry/some-library/some-repo:latest",
				},
				{
					condition: "has an implicit tag and has a digest",
					provided:  "some.registry/some-library/some-repo@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
					does:      "adds an explicit tag and removes the digest",
					expected:  "some.registry/some-library/some-repo:latest",
				},
				{
					condition: "has an explicit tag",
					provided:  "some.registry/some-library/some-repo:some-tag",
					does:      "returns the provided reference",
					expected:  "some.registry/some-library/some-repo:some-tag",
				},
				{
					condition: "has an explicit tag and has a digest",
					provided:  "some.registry/some-library/some-repo:some-tag@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
					does:      "removes the digest",
					expected:  "some.registry/some-library/some-repo:some-tag",
				},
			}
			for _, tc := range testCases {
				tc := tc
				t.Run(tc.condition, func(t *testing.T) {
					t.Run(tc.does, func(t *testing.T) {
						actual := name.ParseMaybe(tc.provided)
						h.AssertEq(t, actual, tc.expected)
					})
				})
			}
		})
	})
}
