//go:build linux

package priv_test

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/priv"
)

func TestRunAs(t *testing.T) {
	if os.Getenv("TEST_RUN_AS_SUBPROCESS") == "1" {
		const targetUID = 65534
		const targetGID = 65534

		if err := priv.RunAs(targetUID, targetGID); err != nil {
			fmt.Printf("RunAs failed: %v\n", err)
			os.Exit(1)
		}

		groups, err := syscall.Getgroups()
		if err != nil {
			fmt.Printf("Getgroups failed: %v\n", err)
			os.Exit(1)
		}

		if len(groups) != 1 || groups[0] != targetGID {
			fmt.Printf("Expected groups [%d], got %v\n", targetGID, groups)
			os.Exit(1)
		}

		os.Exit(0)
	}

	spec.Run(t, "RunAs", testRunAs, spec.Report(report.Terminal{}))
}

func testRunAs(t *testing.T, when spec.G, it spec.S) {
	when("#RunAs", func() {
		it.Before(func() {
			if os.Getuid() != 0 {
				t.Skip("test requires root privileges")
			}
		})

		it("clears supplementary groups so only the target gid remains", func() {
			// use an unprivileged, non-zero uid/gid pair that exists in every
			// standard Linux base image, so the test doesn't depend on a
			// container-specific user; "nobody" is uid/gid 65534 on Debian/Ubuntu/Alpine
			cmd := exec.Command(os.Args[0], "-test.run=^TestRunAs$") //nolint:gosec
			cmd.Env = append(os.Environ(), "TEST_RUN_AS_SUBPROCESS=1")

			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("Subprocess failed: %v\nOutput:\n%s", err, string(out))
			}
		})
	})
}
