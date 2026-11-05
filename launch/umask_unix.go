//go:build unix

package launch

import (
	"fmt"
	"strconv"
	"syscall"

	"github.com/pkg/errors"
)

const (
	umaskEnvVar = "CNB_LAUNCH_UMASK"
)

// SetUmask on unix systems from the value in the `UMASK` environment variable
func SetUmask(env Env) error {
	return SetUmaskWith(env, syscall.Umask)
}

// SetUmaskWith the injected function umaskFn
func SetUmaskWith(env Env, umaskFn func(int) int) error {
	umask := env.Get(umaskEnvVar)
	if umask == "" {
		return nil
	}

	u, err := strconv.ParseInt(umask, 8, 0)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("invalid umask value %s", umask))
	}
	umaskFn(int(u))
	return nil
}
