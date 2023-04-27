package buildpack_test

import (
	"fmt"
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
		tmpDir, err = os.MkdirTemp("", "")
		h.AssertNil(t, err)

		logHandler = memory.New()
		logger = &log.Logger{Handler: logHandler}
	})

	it.After(func() {
		_ = os.RemoveAll(tmpDir)
	})

	when("validating dockerfiles", func() {
		validCases := []string{`
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

		type testCase struct {
			dockerfileContent string
			expectedWarning   string
		}

		warnCases := []testCase{
			{
				dockerfileContent: `CMD ["some-cmd"]`,
				expectedWarning:   "command CMD on line 4 is not recommended",
			},
			{
				dockerfileContent: `MAINTAINER "some-maintainer"`,
				expectedWarning:   "command MAINTAINER on line 4 is not recommended",
			},
			{
				dockerfileContent: `EXPOSE 80/tcp`,
				expectedWarning:   "command EXPOSE on line 4 is not recommended",
			},
			{
				dockerfileContent: `ENTRYPOINT ["some-executable"]`,
				expectedWarning:   "command ENTRYPOINT on line 4 is not recommended",
			},
			{
				dockerfileContent: `VOLUME ["/some-dir"]`,
				expectedWarning:   "command VOLUME on line 4 is not recommended",
			},
			{
				dockerfileContent: `ONBUILD RUN echo "hello" > /world.txt`,
				expectedWarning:   "command ONBUILD on line 4 is not recommended",
			},
			{
				dockerfileContent: `STOPSIGNAL SIGKILL`,
				expectedWarning:   "command STOPSIGNAL on line 4 is not recommended",
			},
			{
				dockerfileContent: `HEALTHCHECK NONE`,
				expectedWarning:   "command HEALTHCHECK on line 4 is not recommended",
			},
		}

		when("build", func() {
			when("valid", func() {
				it("succeeds", func() {
					for i, content := range validCases {
						dockerfileName := fmt.Sprintf("Dockerfile%d", i)
						dockerfilePath := filepath.Join(tmpDir, dockerfileName)
						h.AssertNil(t, os.WriteFile(dockerfilePath, []byte(content), 0600))
						err := buildpack.ValidateBuildDockerfile(dockerfilePath, logger)
						if err != nil {
							t.Fatalf("Error validating Dockerfile %d: %s", i, err)
						}
						h.AssertEq(t, len(logHandler.Entries), 0)
					}
				})

				when("violates SHOULD directives in spec", func() {
					it("succeeds with warning", func() {
						preamble := `
ARG base_image=0
FROM ${base_image}
`
						for i, tc := range warnCases {
							dockerfilePath := filepath.Join(tmpDir, fmt.Sprintf("Dockerfile%d", i))
							h.AssertNil(t, os.WriteFile(dockerfilePath, []byte(preamble+tc.dockerfileContent), 0600))
							logHandler = memory.New()
							logger = &log.Logger{Handler: logHandler}
							err := buildpack.ValidateBuildDockerfile(dockerfilePath, logger)
							h.AssertNil(t, err)
							assertLogEntry(t, logHandler, "build.Dockerfile "+tc.expectedWarning)
						}
					})
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
						h.AssertNil(t, os.WriteFile(dockerfilePath, []byte(tc.dockerfileContent), 0600))
						err := buildpack.ValidateBuildDockerfile(dockerfilePath, logger)
						h.AssertError(t, err, tc.expectedError)
					}
				})
			})
		})

		when("run", func() {
			when("valid", func() {
				it("succeeds", func() {
					for i, content := range validCases {
						dockerfileName := fmt.Sprintf("Dockerfile%d", i)
						dockerfilePath := filepath.Join(tmpDir, dockerfileName)
						h.AssertNil(t, os.WriteFile(dockerfilePath, []byte(content), 0600))
						err := buildpack.ValidateRunDockerfile(&buildpack.DockerfileInfo{Path: dockerfilePath}, logger)
						if err != nil {
							t.Fatalf("Error validating Dockerfile %d: %s", i, err)
						}
						h.AssertEq(t, len(logHandler.Entries), 0)
					}
				})

				when("violates SHOULD directives in spec", func() {
					it("succeeds with warning", func() {
						preamble := `
ARG base_image=0
FROM ${base_image}
`
						for i, tc := range warnCases {
							dockerfilePath := filepath.Join(tmpDir, fmt.Sprintf("Dockerfile%d", i))
							h.AssertNil(t, os.WriteFile(dockerfilePath, []byte(preamble+tc.dockerfileContent), 0600))
							logHandler = memory.New()
							logger = &log.Logger{Handler: logHandler}
							err := buildpack.ValidateRunDockerfile(&buildpack.DockerfileInfo{Path: dockerfilePath}, logger)
							h.AssertNil(t, err)
							assertLogEntry(t, logHandler, "run.Dockerfile "+tc.expectedWarning)
						}
					})
				})

				when("switching the runtime base image", func() {
					it("returns the new base image", func() {
						dockerfilePath := filepath.Join(tmpDir, "run.Dockerfile")
						h.AssertNil(t, os.WriteFile(dockerfilePath, []byte(`FROM some-base-image`), 0600))
						dInfo := &buildpack.DockerfileInfo{Path: dockerfilePath}
						err := buildpack.ValidateRunDockerfile(dInfo, logger)
						h.AssertNil(t, err)
						h.AssertEq(t, dInfo.WithBase, "some-base-image")
						h.AssertEq(t, dInfo.Extend, false)
					})
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
RUN echo "hello" > /world.txt

FROM some-base-image
COPY --from=0 /some-source.txt ./some-dest.txt
`,
							expectedError: "run.Dockerfile is not permitted to use multistage build",
						},
					}
					for i, tc := range testCases {
						dockerfilePath := filepath.Join(tmpDir, fmt.Sprintf("Dockerfile%d", i))
						h.AssertNil(t, os.WriteFile(dockerfilePath, []byte(tc.dockerfileContent), 0600))
						err := buildpack.ValidateRunDockerfile(&buildpack.DockerfileInfo{Path: dockerfilePath}, logger)
						h.AssertError(t, err, tc.expectedError)
					}
				})
			})
		})
	})
}
