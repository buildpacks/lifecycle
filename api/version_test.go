package api_test

import (
	"testing"

	"github.com/buildpacks/lifecycle/api"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestAPIVersion(t *testing.T) {
	t.Parallel()
	t.Run("#Equal", func(t *testing.T) {
		t.Run("is equal to comparison", func(t *testing.T) {
			subject := api.MustParse("0.2")
			comparison := api.MustParse("0.2")

			h.AssertEq(t, subject.Equal(comparison), true)
		})
		t.Run("is not equal to comparison", func(t *testing.T) {
			subject := api.MustParse("0.2")
			comparison := api.MustParse("0.3")

			h.AssertEq(t, subject.Equal(comparison), false)
		})
	})
	t.Run("IsSupersetOf", func(t *testing.T) {
		t.Run("0.x", func(t *testing.T) {
			t.Run("matching Minor value", func(t *testing.T) {
				v := api.MustParse("0.2")
				target := api.MustParse("0.2")

				h.AssertEq(t, v.IsSupersetOf(target), true)
			})
			t.Run("Minor > target Minor", func(t *testing.T) {
				v := api.MustParse("0.2")
				target := api.MustParse("0.1")

				h.AssertEq(t, v.IsSupersetOf(target), false)
			})
			t.Run("Minor < target Minor", func(t *testing.T) {
				v := api.MustParse("0.1")
				target := api.MustParse("0.2")

				h.AssertEq(t, v.IsSupersetOf(target), false)
			})
		})
		t.Run("1.x", func(t *testing.T) {
			t.Run("matching Major and Minor", func(t *testing.T) {
				v := api.MustParse("1.2")
				target := api.MustParse("1.2")

				h.AssertEq(t, v.IsSupersetOf(target), true)
			})
			t.Run("matching Major but Minor > target Minor", func(t *testing.T) {
				v := api.MustParse("1.2")
				target := api.MustParse("1.1")

				h.AssertEq(t, v.IsSupersetOf(target), true)
			})
			t.Run("matching Major but Minor < target Minor", func(t *testing.T) {
				v := api.MustParse("1.1")
				target := api.MustParse("1.2")

				h.AssertEq(t, v.IsSupersetOf(target), false)
			})
			t.Run("Major < target Major", func(t *testing.T) {
				v := api.MustParse("1.0")
				target := api.MustParse("2.0")

				h.AssertEq(t, v.IsSupersetOf(target), false)
			})
			t.Run("Major > target Major", func(t *testing.T) {
				v := api.MustParse("2.0")
				target := api.MustParse("1.0")

				h.AssertEq(t, v.IsSupersetOf(target), false)
			})
		})
	})
	t.Run("#LessThan", func(t *testing.T) {
		var subject = api.MustParse("0.3")
		var toTest = map[string]bool{
			"0.2": false,
			"0.3": false,
			"0.4": true,
		}
		t.Run("returns the expected value", func(t *testing.T) {
			for comparison, expected := range toTest {
				h.AssertEq(t, subject.LessThan(comparison), expected)
			}
		})
	})
	t.Run("#AtLeast", func(t *testing.T) {
		var subject = api.MustParse("0.3")
		var toTest = map[string]bool{
			"0.2": true,
			"0.3": true,
			"0.4": false,
		}
		t.Run("returns the expected value", func(t *testing.T) {
			for comparison, expected := range toTest {
				h.AssertEq(t, subject.AtLeast(comparison), expected)
			}
		})
	})
}
