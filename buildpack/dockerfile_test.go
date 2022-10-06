package buildpack_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/memory"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/buildpack"
	llog "github.com/buildpacks/lifecycle/log"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestDockerfile(t *testing.T) {
	spec.Run(t, "Dockerfile", testDockerfile, spec.Report(report.Terminal{}))
}

func testDockerfile(t *testing.T, when spec.G, it spec.S) {
	var (
		tmpDir     string
		logger     llog.Logger
		logHandler *memory.Handler
	)

	it.Before(func() {
		var err error
		tmpDir, err = ioutil.TempDir("", "")
		h.AssertNil(t, err)

		logHandler = memory.New()
		logger = &log.Logger{Handler: logHandler}
	})

	it.After(func() {
		os.RemoveAll(tmpDir)
	})

	when("verifying dockerfiles", func() {
		when("build", func() {
			when("valid", func() {
				it("succeeds", func() {
					dockerfileContents := []string{
						`
ARG base_image=0
FROM ${base_image}

RUN echo "hello" > /world.txt
`, `
ARG base_image=0
FROM ${base_image}

ADD some-source.txt some-dest.txt

ARG some_arg
RUN echo ${some_arg}

COPY some-source.txt some-dest.txt

ENV SOME_VAR=some-val

LABEL some.label="some-label-dockerfileContent"

USER some-user

WORKDIR some-workdir

SHELL ["/bin/sh"]
`, `
ARG base_image=0
FROM ${base_image}

ARG build_id=0
RUN echo ${build_id}

RUN echo "this statement is never cached"
`,
					}
					for i, content := range dockerfileContents {
						dockerfileName := fmt.Sprintf("Dockerfile%d", i)
						dockerfilePath := filepath.Join(tmpDir, dockerfileName)
						h.AssertNil(t, ioutil.WriteFile(dockerfilePath, []byte(content), 0600))
						err := buildpack.VerifyBuildDockerfile(dockerfilePath, logger)
						if err != nil {
							t.Fatalf("Error verifying Dockerfile %d: %s", i, err)
						}
						h.AssertEq(t, len(logHandler.Entries), 0)
					}
				})
			})

			when("valid, but violates SHOULD directives in spec", func() {
				it("succeeds with warning", func() {
					type testCase struct {
						dockerfileContent string
						expectedWarning   string
					}
					preamble := `
ARG base_image=0
FROM ${base_image}
`
					testCases := []testCase{
						{
							dockerfileContent: preamble + `CMD ["some-cmd"]`,
							expectedWarning:   "build.Dockerfile command CMD on line 4 is not recommended",
						},
						{
							dockerfileContent: preamble + `MAINTAINER "some-maintainer"`,
							expectedWarning:   "build.Dockerfile command MAINTAINER on line 4 is not recommended",
						},
						{
							dockerfileContent: preamble + `EXPOSE 80/tcp`,
							expectedWarning:   "build.Dockerfile command EXPOSE on line 4 is not recommended",
						},
						{
							dockerfileContent: preamble + `ENTRYPOINT ["some-executable"]`,
							expectedWarning:   "build.Dockerfile command ENTRYPOINT on line 4 is not recommended",
						},
						{
							dockerfileContent: preamble + `VOLUME ["/some-dir"]`,
							expectedWarning:   "build.Dockerfile command VOLUME on line 4 is not recommended",
						},
						{
							dockerfileContent: preamble + `ONBUILD RUN echo "hello" > /world.txt`,
							expectedWarning:   "build.Dockerfile command ONBUILD on line 4 is not recommended",
						},
						{
							dockerfileContent: preamble + `STOPSIGNAL SIGKILL`,
							expectedWarning:   "build.Dockerfile command STOPSIGNAL on line 4 is not recommended",
						},
						{
							dockerfileContent: preamble + `HEALTHCHECK NONE`,
							expectedWarning:   "build.Dockerfile command HEALTHCHECK on line 4 is not recommended",
						},
					}
					for i, tc := range testCases {
						dockerfilePath := filepath.Join(tmpDir, fmt.Sprintf("Dockerfile%d", i))
						h.AssertNil(t, ioutil.WriteFile(dockerfilePath, []byte(tc.dockerfileContent), 0600))
						logHandler = memory.New()
						logger = &log.Logger{Handler: logHandler}
						err := buildpack.VerifyBuildDockerfile(dockerfilePath, logger)
						h.AssertNil(t, err)
						assertLogEntry(t, logHandler, tc.expectedWarning)
					}
				})
			})

			when("invalid", func() {
				it("errors", func() {
					type testCase struct {
						dockerfileContent string
						expectedError     string
					}
					testCases := []testCase{
						{
							dockerfileContent: ``,
							expectedError:     "file with no instructions",
						},
						{
							dockerfileContent: `
FROM some-base-image

RUN echo "hello" > /world.txt
`,
							expectedError: "build.Dockerfile did not start with required ARG command",
						},
						{
							dockerfileContent: `
ARG base_image=0
FROM ${base_image}
RUN echo "hello" > /world.txt

FROM some-base-image
COPY --from=0 /some-source.txt ./some-dest.txt
`,
							expectedError: "build.Dockerfile is not permitted to use multistage build",
						},
					}
					for i, tc := range testCases {
						dockerfilePath := filepath.Join(tmpDir, fmt.Sprintf("Dockerfile%d", i))
						h.AssertNil(t, ioutil.WriteFile(dockerfilePath, []byte(tc.dockerfileContent), 0600))
						err := buildpack.VerifyBuildDockerfile(dockerfilePath, logger)
						h.AssertError(t, err, tc.expectedError)
					}
				})
			})
		})

		when("run", func() {
			when("valid", func() {
				it("succeeds", func() {
					dockerfileContents := []string{
						`FROM some-run-image`,
					}
					for i, content := range dockerfileContents {
						dockerfileName := fmt.Sprintf("Dockerfile%d", i)
						dockerfilePath := filepath.Join(tmpDir, dockerfileName)
						h.AssertNil(t, ioutil.WriteFile(dockerfilePath, []byte(content), 0600))
						err := buildpack.VerifyRunDockerfile(dockerfilePath)
						if err != nil {
							t.Fatalf("Error verifying Dockerfile %d: %s", i, err)
						}
						h.AssertEq(t, len(logHandler.Entries), 0)
					}
				})
			})

			when("invalid", func() {
				it("errors", func() {
					type testCase struct {
						dockerfileContent string
						expectedError     string
					}
					testCases := []testCase{
						{
							dockerfileContent: ``,
							expectedError:     "file with no instructions",
						},
						{
							dockerfileContent: `
ARG base_image=0
FROM ${base_image}
`,
							expectedError: "run.Dockerfile should not expect arguments",
						},
						{
							dockerfileContent: `
FROM some-run-image
RUN echo "hello" > /world.txt
`,
							expectedError: "run.Dockerfile is not permitted to have instructions other than FROM",
						},
					}
					for i, tc := range testCases {
						dockerfilePath := filepath.Join(tmpDir, fmt.Sprintf("Dockerfile%d", i))
						h.AssertNil(t, ioutil.WriteFile(dockerfilePath, []byte(tc.dockerfileContent), 0600))
						err := buildpack.VerifyRunDockerfile(dockerfilePath)
						h.AssertError(t, err, tc.expectedError)
					}
				})
			})
		})
	})
}
