package lifecycle

import (
	"path/filepath"
	"os"
	"strings"
	"io/ioutil"
	"fmt"
)

var envMap = map[string][]string{
	"bin": {
		"PATH",
	},
	"lib": {
		"LD_LIBRARY_PATH",
		"LIBRARY_PATH",
	},
	"include": {
		"CPATH",
		"C_INCLUDE_PATH",
		"CPLUS_INCLUDE_PATH",
		"OBJC_INCLUDE_PATH",
	},
	"pkgconfig": {
		"PKG_CONFIG_PATH",
	},
}

type POSIXEnv struct {
	Getenv func(key string) string
	Setenv func(key, value string) error
	Environ func() []string
}

func (p *POSIXEnv) AppendDirs(baseDir string) error {
	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return err
	}
	for dir, vars := range envMap {
		newDir := filepath.Join(absBaseDir, dir)
		if _, err := os.Stat(newDir); err == nil {
			for _, key := range vars {
				value := suffix(p.Getenv(key), ":") + newDir
				if err := p.Setenv(key, value); err != nil {
					return err
				}
			}
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func suffix(s, suffix string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	} else {
		return s + suffix
	}
}

func (p *POSIXEnv) SetEnvDir(envDir string) error {
	envFiles, err := ioutil.ReadDir(envDir)
	if err != nil {
		return err
	}
	for _, f := range envFiles {
		if f.IsDir() {
			return fmt.Errorf("unexpected directory '%s'", f.Name())
		}
		value, err := ioutil.ReadFile(filepath.Join(envDir, f.Name()))
		if err != nil {
			return err
		}
		if err := p.Setenv(f.Name(), string(value)); err != nil {
			return err
		}
	}
	return nil
}

func (p *POSIXEnv) List() []string {
	return p.Environ()
}