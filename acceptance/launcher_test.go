package acceptance

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	h "github.com/buildpacks/lifecycle/testhelpers"
)

var (
	launchImage         = "lifecycle/acceptance/launcher"
	launchDockerContext string
	launcherBinaryDir   string
)

func TestLauncher(t *testing.T) {
	info, err := h.DockerCli(t).Info(context.TODO())
	h.AssertNil(t, err)
	daemonOS = info.OSType

	if daemonOS == "windows" {
		launchDockerContext = filepath.Join("testdata", "launcher", "windows")
		launcherBinaryDir = filepath.Join("testdata", "launcher", "windows", "container", "cnb", "lifecycle")
	} else {
		launchDockerContext = filepath.Join("testdata", "launcher", "posix")
		launcherBinaryDir = filepath.Join("testdata", "launcher", "posix", "container", "cnb", "lifecycle")
	}

	h.MakeAndCopyLauncher(t, daemonOS, launcherBinaryDir)

	h.DockerBuild(t, launchImage, launchDockerContext)
	defer h.DockerImageRemove(t, launchImage)

	spec.Run(t, "acceptance", testLauncher, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testLauncher(t *testing.T, when spec.G, it spec.S) {
	when("there is no CMD provided", func() {
		when("CNB_PROCESS_TYPE is NOT set", func() {
			it("web is the default process-type", func() {
				cmd := exec.Command("docker", "run", "--rm", launchImage)
				assertOutput(t, cmd, "Executing web process-type")
			})
		})

		when("CNB_PROCESS_TYPE is set", func() {
			it("should run the specified CNB_PROCESS_TYPE", func() {
				cmd := exec.Command("docker", "run", "--rm", "--env", "CNB_PROCESS_TYPE=direct-process", launchImage)
				if runtime.GOOS == "windows" {
					assertOutput(t, cmd, "Usage: ping")
				} else {
					assertOutput(t, cmd, "Executing direct-process process-type")
				}
			})
		})
	})

	when("process-type provided in CMD", func() {
		it("launches that process-type", func() {
			cmd := exec.Command("docker", "run", "--rm", launchImage, "direct-process")
			expected := "Executing direct-process process-type"
			if runtime.GOOS == "windows" {
				expected = "Usage: ping"
			}
			assertOutput(t, cmd, expected)
		})

		it("sets env vars from process specific directories", func() {
			cmd := exec.Command("docker", "run", "--rm", launchImage, "worker")
			expected := "worker-process-val"
			assertOutput(t, cmd, expected)
		})
	})

	when("process is direct=false", func() {
		when("the process type has no args", func() {
			it("provides the command directly to bash", func() {
				cmd := exec.Command("docker", "run", "--rm",
					"--env", "VAR1=val1",
					"--env", "VAR2=val with space",
					launchImage, "indirect-process-with-script",
				)
				assertOutput(t, cmd, "'val1' 'val with space'")
			})
		})

		when("the process type has args", func() {
			when("buildpack API 0.4", func() {
				it("command and args become shell-parsed tokens in a script", func() {
					cmd := exec.Command("docker", "run", "--rm",
						"--env", "VAR1=val1",
						"--env", "VAR2=val with space",
						launchImage, "indirect-process-with-args",
					)
					assertOutput(t, cmd, "'val1' 'val with space'")
				})
			})

			when("buildpack API < 0.4", func() {
				it("args become arguments to bash", func() {
					cmd := exec.Command("docker", "run", "--rm",
						launchImage, "legacy-indirect-process-with-args",
					)
					assertOutput(t, cmd, "arg1 arg2")
				})
			})
		})

		it("sources scripts from process specific directories", func() {
			cmd := exec.Command("docker", "run", "--rm", launchImage, "worker")
			expected := "sourced bp profile\nsourced bp worker profile\nsourced app profile"
			assertOutput(t, cmd, expected)
		})
	})

	it("respects CNB_APP_DIR and CNB_LAYERS_DIR environment variables", func() {
		cmd := exec.Command("docker", "run", "--rm",
			"--env", "CNB_APP_DIR=/other-app",
			"--env", "CNB_LAYERS_DIR=/other-layers",
			launchImage)
		assertOutput(t, cmd, "sourced other app profile\nExecuting other-layers web process-type")
	})

	when("provided CMD is not a process-type", func() {
		it("sources profiles and executes the command in a shell", func() {
			cmd := exec.Command("docker", "run", "--rm", launchImage, "echo something")
			assertOutput(t, cmd, "sourced bp profile\nsourced app profile\nsomething")
		})

		it("sets env vars from layers", func() {
			cmd := exec.Command("docker", "run", "--rm", launchImage, "echo $SOME_VAR $OTHER_VAR $WORKER_VAR")
			if runtime.GOOS == "windows" {
				cmd = exec.Command("docker", "run", "--rm", launchImage, "echo %SOME_VAR% %OTHER_VAR% %WORKER_VAR%")
			}
			assertOutput(t, cmd, "sourced bp profile\nsourced app profile\nsome-bp-val other-bp-val worker-no-process-val")
		})

		it("passes through env vars from user, excluding excluded vars", func() {
			args := []string{"echo $SOME_USER_VAR, $CNB_APP_DIR, $OTHER_VAR"}
			if runtime.GOOS == "windows" {
				args = []string{"echo", "%SOME_USER_VAR%, %CNB_APP_DIR%, %OTHER_VAR%"}
			}
			cmd := exec.Command("docker",
				append(
					[]string{
						"run", "--rm",
						"--env", "CNB_APP_DIR=/workspace",
						"--env", "SOME_USER_VAR=some-user-val",
						"--env", "OTHER_VAR=other-user-val",
						launchImage,
					},
					args...)...,
			)

			emptyVar := ""
			if runtime.GOOS == "windows" {
				emptyVar = "%CNB_APP_DIR%"
			}
			assertOutput(t, cmd, fmt.Sprintf("sourced bp profile\nsourced app profile\nsome-user-val, %s, other-user-val**other-bp-val", emptyVar))
		})

		it("adds buildpack bin dirs to the path", func() {
			cmd := exec.Command("docker", "run", "--rm", launchImage, "bp-executable")
			assertOutput(t, cmd, "bp executable")
		})
	})

	when("CMD provided starts with --", func() {
		it("launches command directly", func() {
			if runtime.GOOS == "windows" {
				cmd := exec.Command("docker", "run", "--rm", launchImage, "--", "ping", "/?")
				assertOutput(t, cmd, "Usage: ping")
			} else {
				cmd := exec.Command("docker", "run", "--rm", launchImage, "--", "echo", "something")
				assertOutput(t, cmd, "something")
			}
		})

		it("sets env vars from layers", func() {
			cmd := exec.Command("docker", "run", "--rm", launchImage, "--", "env")
			if runtime.GOOS == "windows" {
				cmd = exec.Command("docker", "run", "--rm", launchImage, "--", "cmd", "/c", "set")
			}

			assertOutput(t, cmd,
				"SOME_VAR=some-bp-val",
				"OTHER_VAR=other-bp-val",
			)
		})

		it("passes through env vars from user, excluding excluded vars", func() {
			cmd := exec.Command("docker", "run", "--rm",
				"--env", "CNB_APP_DIR=/workspace",
				"--env", "SOME_USER_VAR=some-user-val",
				launchImage, "--",
				"env",
			)
			if runtime.GOOS == "windows" {
				cmd = exec.Command("docker", "run", "--rm",
					"--env", "CNB_APP_DIR=/workspace",
					"--env", "SOME_USER_VAR=some-user-val",
					launchImage, "--",
					"cmd", "/c", "set",
				)
			}

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
			cmd := exec.Command("docker", "run", "--rm", launchImage, "--", "bp-executable")
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
