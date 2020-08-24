package env_test

import (
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/buildpacks/lifecycle/env"
	h "github.com/buildpacks/lifecycle/testhelpers"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

func TestPrepare(t *testing.T) {
	spec.Run(t, "Env", testPrepare, spec.Report(report.Terminal{}))
}

func testPrepare(t *testing.T, when spec.G, it spec.S) {
	var (
		getEnvMap = map[string]string{}
		getEnvFn  = func(k string) string {
			return getEnvMap[k]
		}

		setEnvMap = map[string]string{}
		setEnvFn  = func(k, v string) error {
			setEnvMap[k] = v
			return nil
		}

		regEnvMap map[string]string
	)

	when("#PrepareWindowsOSEnv", func() {
		when("reg and env values differ", func() {
			it("prepends reg values to env PATH and PATHEXT", func() {
				regEnvMap = map[string]string{
					"PATH":     pathList(`c\reg-first`, `c\reg-second`),
					"PATHEXT":  pathList(`.REG1`, `.REG2`),
					"BOTH":     `reg-ignored`,
					"REG_ONLY": `reg-only`,
					"EMPTY":    ``,
				}
				getEnvMap = map[string]string{
					"PATH":     pathList(`c\env-first`, `c\env-second`),
					"PATHEXT":  pathList(`.ENV1`, `.ENV2`),
					"BOTH":     `env-wins`,
					"ENV_ONLY": `env-only`,
					"EMPTY":    ``,
				}

				h.AssertNil(t, env.PrepareWindowsOSEnv(regEnvMap, getEnvFn, setEnvFn))

				expectedSetEnvKV := map[string]string{
					"PATH":     pathList(`c\reg-first`, `c\reg-second`, `c\env-first`, `c\env-second`),
					"PATHEXT":  pathList(`.REG1`, `.REG2`, `.ENV1`, `.ENV2`),
					"REG_ONLY": `reg-only`,
				}
				h.AssertEq(t, setEnvMap, expectedSetEnvKV)
			})
		})

		when("reg and env values are identical", func() {
			it("no change", func() {
				regEnvMap = map[string]string{
					"PATH":    pathList(`c\both-first`, `c\both-second`),
					"PATHEXT": pathList(`.BOTH1`, `.BOTH2`),
					"BOTH":    `tie`,
					"EMPTY":   ``,
				}
				getEnvMap = map[string]string{
					"PATH":    pathList(`c\both-first`, `c\both-second`),
					"PATHEXT": pathList(`.BOTH1`, `.BOTH2`),
					"BOTH":    `tie`,
					"EMPTY":   ``,
				}
				h.AssertNil(t, env.PrepareWindowsOSEnv(regEnvMap, getEnvFn, setEnvFn))

				// nothing needs to be set
				h.AssertEq(t, setEnvMap, map[string]string{})
			})
		})

		when("values have different case", func() {
			it("de-dupe path values, preferring reg case, otherwise don't consider case", func() {
				regEnvMap = map[string]string{
					"PATH":    pathList(`C\BOTH-FIRST`, `c\both-second`, `c\REG-third`),
					"PATHEXT": pathList(`.BOTH1`, `.both2`, `.REG3`),
					"BOTH":    `TIE`,
					"EMPTY":   ``,
				}
				getEnvMap = map[string]string{
					"PATH":    pathList(`c\both-first`, `C\BOTH-SECOND`, `c\ENV-third`),
					"PATHEXT": pathList(`.both1`, `.BOTH2`, `.ENV3`),
					"BOTH":    `tie`,
					"EMPTY":   ``,
				}
				h.AssertNil(t, env.PrepareWindowsOSEnv(regEnvMap, getEnvFn, setEnvFn))

				expectedSetEnvMap := map[string]string{
					"PATH":    pathList(`C\BOTH-FIRST`, `c\both-second`, `c\REG-third`, `c\ENV-third`),
					"PATHEXT": pathList(`.BOTH1`, `.both2`, `.REG3`, `.ENV3`),
				}
				h.AssertEq(t, setEnvMap, expectedSetEnvMap)
			})
		})
	})

	when("#WindowsRegistryEnvMap", func() {
		it.Before(func() {
			h.SkipIf(t, runtime.GOOS != "windows", "uses windows-only library")
		})

		it("returns registry env vars", func() {
			regEnvMap, err := env.WindowsRegistryEnvMap()
			h.AssertNil(t, err)
			h.AssertStringContains(t, strings.ToLower(regEnvMap["PATH"]), `c:\windows\system32;`)
		})
	})
}

func pathList(pathParts ...string) string {
	return strings.Join(pathParts, string(os.PathListSeparator))
}
