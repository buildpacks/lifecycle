// +build windows

package launch_test

import "testing"

func syscallExecWithStdout(t *testing.T, tmpDir string) func(argv0 string, argv []string, envv []string) error {
	return nil
}
