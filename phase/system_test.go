package phase

import (
	"os"
	"testing"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/log"
	"github.com/buildpacks/lifecycle/platform/files"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestMergeSystemBuildpacks(t *testing.T) {
	logger := log.NewDefaultLogger(os.Stdout)

	t.Run("merges pre and post buildpacks", func(t *testing.T) {
		order := buildpack.Order{
			{Group: []buildpack.GroupElement{
				{ID: "order-bp-1", Version: "1.0.0"},
				{ID: "order-bp-2", Version: "2.0.0"},
			}},
			{Group: []buildpack.GroupElement{
				{ID: "order-bp-3", Version: "3.0.0"},
			}},
		}

		system := files.System{
			Pre: files.SystemBuildpacks{
				Buildpacks: []files.SystemBuildpack{
					{ID: "pre-bp-1", Version: "0.1.0"},
					{ID: "pre-bp-2", Version: "0.2.0"},
				},
			},
			Post: files.SystemBuildpacks{
				Buildpacks: []files.SystemBuildpack{
					{ID: "post-bp-1", Version: "9.0.0"},
				},
			},
		}

		merged := mergeSystemBuildpacks(order, system, logger)

		// Should have same number of groups
		h.AssertEq(t, len(merged), 2)

		// First group should have: pre-bp-1, pre-bp-2, order-bp-1, order-bp-2, post-bp-1
		h.AssertEq(t, len(merged[0].Group), 5)
		h.AssertEq(t, merged[0].Group[0].ID, "pre-bp-1")
		h.AssertEq(t, merged[0].Group[0].Version, "0.1.0")
		h.AssertEq(t, merged[0].Group[1].ID, "pre-bp-2")
		h.AssertEq(t, merged[0].Group[1].Version, "0.2.0")
		h.AssertEq(t, merged[0].Group[2].ID, "order-bp-1")
		h.AssertEq(t, merged[0].Group[2].Version, "1.0.0")
		h.AssertEq(t, merged[0].Group[3].ID, "order-bp-2")
		h.AssertEq(t, merged[0].Group[3].Version, "2.0.0")
		h.AssertEq(t, merged[0].Group[4].ID, "post-bp-1")
		h.AssertEq(t, merged[0].Group[4].Version, "9.0.0")

		// Second group should have: pre-bp-1, pre-bp-2, order-bp-3, post-bp-1
		h.AssertEq(t, len(merged[1].Group), 4)
		h.AssertEq(t, merged[1].Group[0].ID, "pre-bp-1")
		h.AssertEq(t, merged[1].Group[1].ID, "pre-bp-2")
		h.AssertEq(t, merged[1].Group[2].ID, "order-bp-3")
		h.AssertEq(t, merged[1].Group[3].ID, "post-bp-1")
	})

	t.Run("handles only pre buildpacks", func(t *testing.T) {
		order := buildpack.Order{
			{Group: []buildpack.GroupElement{
				{ID: "order-bp", Version: "1.0.0"},
			}},
		}

		system := files.System{
			Pre: files.SystemBuildpacks{
				Buildpacks: []files.SystemBuildpack{
					{ID: "pre-bp", Version: "0.5.0"},
				},
			},
		}

		merged := mergeSystemBuildpacks(order, system, logger)

		h.AssertEq(t, len(merged), 1)
		h.AssertEq(t, len(merged[0].Group), 2)
		h.AssertEq(t, merged[0].Group[0].ID, "pre-bp")
		h.AssertEq(t, merged[0].Group[1].ID, "order-bp")
	})

	t.Run("handles only post buildpacks", func(t *testing.T) {
		order := buildpack.Order{
			{Group: []buildpack.GroupElement{
				{ID: "order-bp", Version: "1.0.0"},
			}},
		}

		system := files.System{
			Post: files.SystemBuildpacks{
				Buildpacks: []files.SystemBuildpack{
					{ID: "post-bp", Version: "5.0.0"},
				},
			},
		}

		merged := mergeSystemBuildpacks(order, system, logger)

		h.AssertEq(t, len(merged), 1)
		h.AssertEq(t, len(merged[0].Group), 2)
		h.AssertEq(t, merged[0].Group[0].ID, "order-bp")
		h.AssertEq(t, merged[0].Group[1].ID, "post-bp")
	})

	t.Run("returns unchanged order when system is empty", func(t *testing.T) {
		order := buildpack.Order{
			{Group: []buildpack.GroupElement{
				{ID: "order-bp", Version: "1.0.0"},
			}},
		}

		system := files.System{}

		merged := mergeSystemBuildpacks(order, system, logger)

		h.AssertEq(t, len(merged), 1)
		h.AssertEq(t, len(merged[0].Group), 1)
		h.AssertEq(t, merged[0].Group[0].ID, "order-bp")
	})

	t.Run("preserves group extensions", func(t *testing.T) {
		order := buildpack.Order{
			{
				Group: []buildpack.GroupElement{
					{ID: "order-bp", Version: "1.0.0"},
				},
				GroupExtensions: []buildpack.GroupElement{
					{ID: "ext-1", Version: "1.0.0", Extension: true},
				},
			},
		}

		system := files.System{
			Pre: files.SystemBuildpacks{
				Buildpacks: []files.SystemBuildpack{
					{ID: "pre-bp", Version: "0.5.0"},
				},
			},
		}

		merged := mergeSystemBuildpacks(order, system, logger)

		h.AssertEq(t, len(merged), 1)
		h.AssertEq(t, len(merged[0].Group), 2)
		h.AssertEq(t, len(merged[0].GroupExtensions), 1)
		h.AssertEq(t, merged[0].GroupExtensions[0].ID, "ext-1")
	})
}

func TestConvertSystemToGroupElements(t *testing.T) {
	t.Run("converts system buildpacks to group elements", func(t *testing.T) {
		systemBps := []files.SystemBuildpack{
			{ID: "bp-1", Version: "1.0.0"},
			{ID: "bp-2", Version: "2.0.0"},
		}

		elements := convertSystemToGroupElements(systemBps)

		h.AssertEq(t, len(elements), 2)
		h.AssertEq(t, elements[0].ID, "bp-1")
		h.AssertEq(t, elements[0].Version, "1.0.0")
		h.AssertEq(t, elements[1].ID, "bp-2")
		h.AssertEq(t, elements[1].Version, "2.0.0")
	})

	t.Run("handles empty list", func(t *testing.T) {
		systemBps := []files.SystemBuildpack{}

		elements := convertSystemToGroupElements(systemBps)

		h.AssertEq(t, len(elements), 0)
	})
}
