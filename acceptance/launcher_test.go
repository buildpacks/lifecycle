package acceptance

import (
	"context"
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
	when("Buildpack API >= 0.5", func() {
		when("exec.d", func() {
			it("executes the binaries and modifies env before running profiles", func() {
				cmd := exec.Command("docker", "run", "--rm",
					"--env=VAR_FROM_EXEC_D=ORIG_VAL",
					launchImage, "exec.d-checker")
				expected := "/layers/0.5_buildpack/some_layer/exec.d/helper was executed\n"
				expected += "Exec.d Working Dir: /workspace\n"
				expected += "/layers/0.5_buildpack/some_layer/exec.d/exec.d-checker/helper was executed\n"
				expected += "Exec.d Working Dir: /workspace\n"
				expected += "sourced bp profile\n"
				expected += "sourced app profile\n"
				expected += "VAR_FROM_EXEC_D: ORIG_VAL:VAL_FROM_EXEC_D:VAL_FROM_EXEC_D"
				assertOutput(t, cmd, expected)
			})
		})
	})

	when("Platform API >= 0.4", func() {
		when("entrypoint is a process", func() {
			it("launches that process", func() {
				cmd := exec.Command("docker", "run", "--rm",
					"--entrypoint=web",
					"--env=CNB_PLATFORM_API=0.4",
					launchImage)
				assertOutput(t, cmd, "Executing web process-type")
			})

			it("appends any args to the process args", func() {
				cmd := exec.Command("docker", "run", "--rm",
					"--entrypoint=web",
					"--env=CNB_PLATFORM_API=0.4",
					launchImage, "with user provided args")
				if runtime.GOOS == "windows" {
					assertOutput(t, cmd, `Executing web process-type "with user provided args"`)
				} else {
					assertOutput(t, cmd, "Executing web process-type with user provided args")
				}
			})
		})

		when("entrypoint is a not a process", func() {
			it("builds a process from the arguments", func() {
				cmd := exec.Command("docker", "run", "--rm",
					"--entrypoint=launcher",
					"--env=CNB_PLATFORM_API=0.4",
					launchImage, "--", "env")
				if runtime.GOOS == "windows" {
					cmd = exec.Command("docker", "run", "--rm",
						`--entrypoint=launcher`,
						"--env=CNB_PLATFORM_API=0.4",
						launchImage, "--", "cmd", "/c", "set",
					)
				}

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
					"--env=CNB_PLATFORM_API=0.4",
					"--env=CNB_NO_COLOR=true",
					launchImage,
				)
				out, err := cmd.CombinedOutput()
				h.AssertNotNil(t, err)
				h.AssertStringContains(t, string(out), "Warning: CNB_PROCESS_TYPE is not supported in Platform API 0.4")
				h.AssertStringContains(t, string(out), `Warning: Run with ENTRYPOINT 'direct-process' to invoke the 'direct-process' process type`)
				h.AssertStringContains(t, string(out), "ERROR: failed to launch: determine start command: when there is no default process a command is required")
			})
		})
	})

	when("Platform API < 0.4", func() {
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
				it("runs command as script", func() {
					h.SkipIf(t, runtime.GOOS == "windows", "scripts are unsupported on windows")
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
					// buildpack API is determined by looking up the API of the process buildpack in metadata.toml

					it("command and args become shell-parsed tokens in a script", func() {
						var val2 string
						if runtime.GOOS == "windows" {
							val2 = `"val with space"` //windows values with spaces must contain quotes
						} else {
							val2 = "val with space"
						}
						cmd := exec.Command("docker", "run", "--rm",
							"--env", "VAR1=val1",
							"--env", "VAR2="+val2,
							launchImage, "indirect-process-with-args",
						)
						assertOutput(t, cmd, "'val1' 'val with space'")
					})
				})

				when("buildpack API < 0.4", func() {
					// buildpack API is determined by looking up the API of the process buildpack in metadata.toml

					it("args become arguments to bash", func() {
						h.SkipIf(t, runtime.GOOS == "windows", "scripts are unsupported on windows")
						cmd := exec.Command("docker", "run", "--rm",
							launchImage, "legacy-indirect-process-with-args",
						)
						assertOutput(t, cmd, "'arg' 'arg with spaces'")
					})

					it("script must be explicitly written to accept bash args", func() {
						h.SkipIf(t, runtime.GOOS == "windows", "scripts are unsupported on windows")
						cmd := exec.Command("docker", "run", "--rm",
							launchImage, "legacy-indirect-process-with-incorrect-args",
						)
						output, err := cmd.CombinedOutput()
						h.AssertNotNil(t, err)
						h.AssertStringContains(t, string(output), "printf: usage: printf [-v var] format [arguments]")
					})
				})
			})

			it("sources scripts from process specific directories", func() {
				cmd := exec.Command("docker", "run", "--rm", launchImage, "profile-checker")
				expected := "sourced bp profile\nsourced bp profile-checker profile\nsourced app profile\nval-from-profile"
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
				cmd := exec.Command("docker", "run", "--rm", launchImage, "echo", "something")
				assertOutput(t, cmd, "sourced bp profile\nsourced app profile\nsomething")
			})

			it("sets env vars from layers", func() {
				cmd := exec.Command("docker", "run", "--rm", launchImage, "echo", "$SOME_VAR", "$OTHER_VAR", "$WORKER_VAR")
				if runtime.GOOS == "windows" {
					cmd = exec.Command("docker", "run", "--rm", launchImage, "echo", "%SOME_VAR%", "%OTHER_VAR%", "%WORKER_VAR%")
				}
				assertOutput(t, cmd, "sourced bp profile\nsourced app profile\nsome-bp-val other-bp-val worker-no-process-val")
			})

			it("passes through env vars from user, excluding excluded vars", func() {
				args := []string{"echo", "$SOME_USER_VAR, $CNB_APP_DIR, $OTHER_VAR"}
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

				if runtime.GOOS == "windows" {
					// windows values with spaces will contain quotes
					// empty values on windows preserve variable names instead of interpolating to empty strings
					assertOutput(t, cmd, "sourced bp profile\nsourced app profile\n\"some-user-val, %CNB_APP_DIR%, other-user-val**other-bp-val\"")
				} else {
					assertOutput(t, cmd, "sourced bp profile\nsourced app profile\nsome-user-val, , other-user-val**other-bp-val")
				}
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
