package acceptance

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	h "github.com/buildpacks/lifecycle/testhelpers"
)

var (
	launchImage  string
	launcherPath string
	launchTest   *PhaseTest
)

func TestLauncher(t *testing.T) {
	testImageDockerContext := filepath.Join("testdata", "launcher")
	launchTest = NewPhaseTest(t, "launcher", testImageDockerContext, withoutDaemonFixtures, withoutRegistry)

	containerBinaryDir := filepath.Join("testdata", "launcher", "linux", "container", "cnb", "lifecycle")
	withCustomContainerBinaryDir := func(_ *testing.T, phaseTest *PhaseTest) {
		phaseTest.containerBinaryDir = containerBinaryDir
	}
	launchTest.Start(t, withCustomContainerBinaryDir)
	t.Cleanup(func() { launchTest.Stop(t) })

	launchImage = launchTest.testImageRef
	launcherPath = launchTest.containerBinaryPath

	spec.Run(t, "acceptance", testLauncher, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testLauncher(t *testing.T, when spec.G, it spec.S) {
	when("exec.d", func() {
		it("executes the binaries and modifies env before running profiles", func() {
			cmd := exec.Command("docker", "run", "--rm", //nolint
				"--env=CNB_PLATFORM_API=0.7",
				"--entrypoint=exec.d-checker"+exe,
				"--env=VAR_FROM_EXEC_D=orig-val",
				launchImage)

			helper := "helper" + exe
			execDHelper := ctrPath("/layers", execDBpDir, "some_layer/exec.d", helper)
			execDCheckerHelper := ctrPath("/layers", execDBpDir, "some_layer/exec.d/exec.d-checker", helper)
			workDir := ctrPath("/workspace")

			expected := fmt.Sprintf("%s was executed\n", execDHelper)
			expected += fmt.Sprintf("Exec.d Working Dir: %s\n", workDir)
			expected += fmt.Sprintf("%s was executed\n", execDCheckerHelper)
			expected += fmt.Sprintf("Exec.d Working Dir: %s\n", workDir)
			expected += "sourced bp profile\n"
			expected += "sourced app profile\n"
			expected += "VAR_FROM_EXEC_D: orig-val:val-from-exec.d:val-from-exec.d-for-process-type-exec.d-checker"

			assertOutput(t, cmd, expected)
		})
	})

	when("entrypoint is a process", func() {
		it("launches that process", func() {
			cmd := exec.Command("docker", "run", "--rm", //nolint
				"--entrypoint=web",
				"--env=CNB_PLATFORM_API="+latestPlatformAPI,
				launchImage)
			assertOutput(t, cmd, "Executing web process-type")
		})

		when("process contains a period", func() {
			it("launches that process", func() {
				cmd := exec.Command("docker", "run", "--rm",
					"--entrypoint=process.with.period"+exe,
					"--env=CNB_PLATFORM_API="+latestPlatformAPI,
					launchImage)
				assertOutput(t, cmd, "Executing process.with.period process-type")
			})
		})

		it("appends any args to the process args", func() {
			cmd := exec.Command( //nolint
				"docker", "run", "--rm",
				"--entrypoint=web",
				"--env=CNB_PLATFORM_API="+latestPlatformAPI,
				launchImage, "with user provided args",
			)
			assertOutput(t, cmd, "Executing web process-type with user provided args")
		})
	})

	when("entrypoint is a not a process", func() {
		it("builds a process from the arguments", func() {
			cmd := exec.Command( //nolint
				"docker", "run", "--rm",
				"--entrypoint=launcher",
				"--env=CNB_PLATFORM_API="+latestPlatformAPI,
				launchImage, "--",
				"env",
			)

			assertOutput(t, cmd,
				"SOME_VAR=some-bp-val",
				"OTHER_VAR=other-bp-val",
			)
		})
	})

	when("CNB_PROCESS_TYPE is set", func() {
		it("should warn", func() {
			cmd := exec.Command("docker", "run", "--rm",
				"--env=CNB_PROCESS_TYPE=direct-process",
				"--env=CNB_PLATFORM_API="+latestPlatformAPI,
				"--env=CNB_NO_COLOR=true",
				launchImage,
			)
			out, err := cmd.CombinedOutput()
			h.AssertNotNil(t, err)
			h.AssertStringContains(t, string(out), "Warning: CNB_PROCESS_TYPE is not supported in Platform API "+latestPlatformAPI)
			h.AssertStringContains(t, string(out), `Warning: Run with ENTRYPOINT 'direct-process' to invoke the 'direct-process' process type`)
			h.AssertStringContains(t, string(out), "ERROR: failed to launch: determine start command: when there is no default process a command is required")
		})
	})

	when("provided CMD is not a process-type", func() {
		it("sources profiles and executes the command in a shell", func() {
			cmd := exec.Command( //nolint
				"docker", "run", "--rm",
				"--env=CNB_PLATFORM_API="+latestPlatformAPI,
				launchImage,
				"echo", "something",
			)
			assertOutput(t, cmd, "sourced bp profile\nsourced app profile\nsomething")
		})

		it("sets env vars from layers", func() {
			cmd := exec.Command( //nolint
				"docker", "run", "--rm",
				"--env=CNB_PLATFORM_API="+latestPlatformAPI,
				launchImage,
				"echo", "$SOME_VAR", "$OTHER_VAR", "$WORKER_VAR",
			)
			assertOutput(t, cmd, "sourced bp profile\nsourced app profile\nsome-bp-val other-bp-val worker-no-process-val")
		})

		it("passes through env vars from user, excluding excluded vars", func() {
			args := []string{"echo", "$SOME_USER_VAR, $CNB_APP_DIR, $OTHER_VAR"}
			cmd := exec.Command("docker",
				append(
					[]string{
						"run", "--rm",
						"--env", "CNB_APP_DIR=" + ctrPath("/workspace"),
						"--env=CNB_PLATFORM_API=" + latestPlatformAPI,
						"--env", "SOME_USER_VAR=some-user-val",
						"--env", "OTHER_VAR=other-user-val",
						launchImage,
					},
					args...)...,
			) // #nosec G204

			assertOutput(t, cmd, "sourced bp profile\nsourced app profile\nsome-user-val, , other-user-val**other-bp-val")
		})

		it("adds buildpack bin dirs to the path", func() {
			cmd := exec.Command( //nolint
				"docker", "run", "--rm",
				"--env=CNB_PLATFORM_API="+latestPlatformAPI,
				launchImage,
				"bp-executable",
			)
			assertOutput(t, cmd, "bp executable")
		})
	})

	when("CMD provided starts with --", func() {
		it("launches command directly", func() {
			cmd := exec.Command( //nolint
				"docker", "run", "--rm",
				"--env=CNB_PLATFORM_API="+latestPlatformAPI,
				launchImage, "--",
				"echo", "something",
			)
			assertOutput(t, cmd, "something")
		})

		it("sets env vars from layers", func() {
			cmd := exec.Command( //nolint
				"docker", "run", "--rm",
				"--env=CNB_PLATFORM_API="+latestPlatformAPI,
				launchImage, "--",
				"env",
			)

			assertOutput(t, cmd,
				"SOME_VAR=some-bp-val",
				"OTHER_VAR=other-bp-val",
			)
		})

		it("passes through env vars from user, excluding excluded vars", func() {
			cmd := exec.Command( //nolint
				"docker", "run", "--rm",
				"--env", "CNB_APP_DIR=/workspace",
				"--env=CNB_PLATFORM_API="+latestPlatformAPI,
				"--env", "SOME_USER_VAR=some-user-val",
				launchImage, "--",
				"env",
			)

			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("failed to run %v\n OUTPUT: %s\n ERROR: %s\n", cmd.Args, output, err)
			}
			expected := "SOME_USER_VAR=some-user-val"
			if !strings.Contains(string(output), expected) {
				t.Fatalf("failed to execute provided CMD:\n\t got: %s\n\t want: %s", output, expected)
			}

			if strings.Contains(string(output), "CNB_APP_DIR") {
				t.Fatalf("env contained white listed env far CNB_APP_DIR:\n\t got: %s\n", output)
			}
		})

		it("adds buildpack bin dirs to the path before looking up command", func() {
			cmd := exec.Command( //nolint
				"docker", "run", "--rm",
				"--env=CNB_PLATFORM_API="+latestPlatformAPI,
				launchImage, "--",
				"bp-executable",
			)
			assertOutput(t, cmd, "bp executable")
		})
	})
}

func assertOutput(t *testing.T, cmd *exec.Cmd, expected ...string) {
	t.Helper()
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to run %v\n OUTPUT: %s\n ERROR: %s\n", cmd.Args, output, err)
	}
	for _, ex := range expected {
		if !strings.Contains(strings.ReplaceAll(string(output), "\r\n", "\n"), ex) {
			t.Fatalf("failed:\n\t output: %s\n\t should include: %s", output, ex)
		}
	}
}
