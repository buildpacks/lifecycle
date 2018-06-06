package lifecycle

import (
	"io/ioutil"
	"os"
	"path/filepath"
)

type Env struct {
	Getenv  func(key string) string
	Setenv  func(key, value string) error
	Environ func() []string
	Map     map[string][]string
}

func (p *Env) AddRootDir(baseDir string) error {
	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return err
	}
	for dir, vars := range p.Map {
		newDir := filepath.Join(absBaseDir, dir)
		if _, err := os.Stat(newDir); err == nil {
			for _, key := range vars {
				value := suffix(p.Getenv(key), os.PathListSeparator) + newDir
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

func suffix(s string, suffix byte) string {
	if s == "" {
		return ""
	} else {
		return s + string(suffix)
	}
}

func (p *Env) SetEnvDir(envDir string) error {
	return eachEnvFile(envDir, func(k, v string) error {
		return p.Setenv(k, v)
	})
}

func (p *Env) AddEnvDir(envDir string) error {
	return eachEnvFile(envDir, func(k, v string) error {
		return p.Setenv(k, suffix(p.Getenv(k), os.PathListSeparator)+string(v))
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

func (p *Env) List() []string {
	return p.Environ()
}
