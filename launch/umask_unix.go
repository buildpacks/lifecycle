//go:build unix

package launch

import (
	"strconv"
	"syscall"

	"github.com/pkg/errors"
)

// SetUmask on unix systems from the value in the `UMASK` environment variable
func SetUmask(env Env) error {
	return SetUmaskWith(env, syscall.Umask)
}

// SetUmaskWith the injected function umaskFn
func SetUmaskWith(env Env, umaskFn func(int) int) error {
	if umask := env.Get("UMASK"); umask != "" {
		u, err := strconv.ParseInt(umask, 8, 0)
		if err != nil {
			return errors.Wrap(err, "invalid umask value")
		}
		umaskFn(int(u))
	}
	return nil
}
