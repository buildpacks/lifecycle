package launch_test

import (
	"bytes"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/launch/testmock"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestExecD(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "ExecD", testExecD, spec.Sequential(), spec.Report(report.Terminal{}))
}

func testExecD(t *testing.T, when spec.G, it spec.S) {
	when("ExecD", func() {
		var (
			path        string
			tmpDir      string
			mockCtrl    *gomock.Controller
			env         *testmock.MockEnv
			runner      launch.ExecDRunner
			out, errOut bytes.Buffer
		)

		it.Before(func() {
			mockCtrl = gomock.NewController(t)
			env = testmock.NewMockEnv(mockCtrl)

			var err error
			tmpDir, err = ioutil.TempDir("", "test-execd")
			h.AssertNil(t, err)
			wd, err := os.Getwd()
			h.AssertNil(t, err)
			path = filepath.Join(tmpDir, "execd")
			cmd := exec.Command("go", "build",
				"-o", path,
				filepath.Join(wd, "testdata", "cmd", "execd"),
			)
			output, err := cmd.Output()
			if err != nil {
				t.Fatalf("Failed to build test execd binary\n output: %s\n error: %s", output, err)
			}
			runner = launch.ExecDRunner{
				Out: &out,
				Err: &errOut,
			}
		})

		it.After(func() {
			h.AssertNil(t, os.RemoveAll(tmpDir))
			mockCtrl.Finish()
		})

		when("unix", func() {
			it.Before(func() {
				h.SkipIf(t, runtime.GOOS == "windows", "exec.d is not yet supported on windows")
			})

			it("modifies the env", func() {
				env.EXPECT().List().Return([]string{})
				env.EXPECT().Set("APPEND_VAR", "SOME_VAL")
				env.EXPECT().Set("OTHER_VAR", "OTHER_VAL")
				h.AssertNil(t, runner.ExecD(path, env))
			})

			it("receives the env", func() {
				env.EXPECT().List().Return([]string{
					"APPEND_VAR=ORIG_VAL",
				})
				env.EXPECT().Set("APPEND_VAR", "ORIG_VAL|SOME_VAL")
				env.EXPECT().Set("OTHER_VAR", "OTHER_VAL")
				h.AssertNil(t, runner.ExecD(path, env))
			})

			it("sets stdout to out", func() {
				env.EXPECT().List().Return([]string{})
				env.EXPECT().Set(gomock.Any(), gomock.Any()).AnyTimes()
				h.AssertNil(t, runner.ExecD(path, env))
				h.AssertEq(t, out.String(), "stdout from execd\n")
			})

			it("sets stderr to err", func() {
				env.EXPECT().List().Return([]string{})
				env.EXPECT().Set(gomock.Any(), gomock.Any()).AnyTimes()
				h.AssertNil(t, runner.ExecD(path, env))
				h.AssertEq(t, errOut.String(), "stderr from execd\n")
			})
		})

		when("windows", func() {
			it.Before(func() {
				h.SkipIf(t, runtime.GOOS != "windows", "exec.d is not yet supported on windows")
			})

			it("errors", func() {
				h.AssertError(t, runner.ExecD(path, env), "exec.d is not currently supported on windows")
			})
		})
	})
}
