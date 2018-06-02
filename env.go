package lifecycle

import (
	"io/ioutil"
	"os"
	"path/filepath"
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
	Getenv  func(key string) string
	Setenv  func(key, value string) error
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
	if s == "" {
		return ""
	} else {
		return s + suffix
	}
}

func (p *POSIXEnv) SetEnvDir(envDir string) error {
	return eachEnvFile(envDir, func(k, v string) error {
		return p.Setenv(k, v)
	})
}

func (p *POSIXEnv) AddEnvDir(envDir string) error {
	return eachEnvFile(envDir, func(k, v string) error {
		return p.Setenv(k, suffix(p.Getenv(k), ":")+string(v))
	})
}

func eachEnvFile(dir string, fn func(k, v string) error) error {
	files, err := ioutil.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		value, err := ioutil.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			return err
		}
		if err := fn(f.Name(), string(value)); err != nil {
			return err
		}
	}
	return nil
}

func (p *POSIXEnv) List() []string {
	return p.Environ()
}
