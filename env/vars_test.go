package env_test

import (
	"testing"

	"github.com/buildpacks/lifecycle/env"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestVars(t *testing.T) {
	t.Run("#NewVars", func(t *testing.T) {
		t.Run("case sensitive", func(t *testing.T) {
			t.Run("should load values as is", func(t *testing.T) {
				m := env.NewVars(
					map[string]string{
						"foo": "bar",
					},
					false,
				)

				h.AssertEq(t, m.Get("foo"), "bar")
				h.AssertEq(t, m.Get("Foo"), "")
			})
		})
		t.Run("case insensitive", func(t *testing.T) {
			t.Run("should load values normalized", func(t *testing.T) {
				m := env.NewVars(
					map[string]string{
						"foo": "bar",
					},
					true,
				)

				h.AssertEq(t, m.Get("foo"), "bar")
				h.AssertEq(t, m.Get("Foo"), "bar")
			})
		})
	})
	t.Run("#Set", func(t *testing.T) {
		t.Run("case sensitive", func(t *testing.T) {
			t.Run("should set value as is", func(t *testing.T) {
				m := env.NewVars(nil, false)
				m.Set("foo", "bar")

				h.AssertEq(t, m.Get("foo"), "bar")
				h.AssertEq(t, m.Get("Foo"), "")
			})
		})
		t.Run("case insensitive", func(t *testing.T) {
			t.Run("should set value normalized", func(t *testing.T) {
				m := env.NewVars(nil, true)
				m.Set("foo", "bar")

				h.AssertEq(t, m.Get("foo"), "bar")
				h.AssertEq(t, m.Get("Foo"), "bar")
			})
		})
	})
	t.Run("#Values", func(t *testing.T) {
		t.Run("case sensitive", func(t *testing.T) {
			t.Run("should load values as is", func(t *testing.T) {
				m := env.NewVars(
					map[string]string{
						"foo": "bar",
						"baz": "taz",
					},
					false,
				)

				h.AssertContains(t, m.List(), "foo=bar", "baz=taz")
			})
		})
		t.Run("case insensitive", func(t *testing.T) {
			t.Run("should load values normalized", func(t *testing.T) {
				m := env.NewVars(
					map[string]string{
						"foo": "bar",
						"baz": "taz",
					},
					true,
				)

				h.AssertContains(t, m.List(), "FOO=bar", "BAZ=taz")
			})
		})
	})
}
