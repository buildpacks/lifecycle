package env

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

type Env struct {
	RootDirMap map[string][]string
	Vars       *Vars
}

func varsFromEnviron(environ []string, ignoreCase bool, removeKey func(string) bool) *Vars {
	vars := NewVars(nil, ignoreCase)
	for _, kv := range environ {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 {
			continue
		}
		if removeKey(parts[0]) {
			continue
		}
		vars.Set(parts[0], parts[1])
	}
	return vars
}

func (p *Env) AddRootDir(baseDir string) error {
	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return err
	}
	for dir, vars := range p.RootDirMap {
		newDir := filepath.Join(absBaseDir, dir)
		if _, err := os.Stat(newDir); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		for _, key := range vars {
			p.Vars.Set(key, newDir+prefix(p.Vars.Get(key), os.PathListSeparator))
		}
	}
	return nil
}

func (p *Env) AddEnvDir(envDir string) error {
	return eachEnvFile(envDir, func(k, v string) error {
		parts := strings.SplitN(k, ".", 2)
		name := parts[0]
		var action string
		if len(parts) > 1 {
			action = parts[1]
		}
		switch action {
		case "prepend":
			p.Vars.Set(name, v+prefix(p.Vars.Get(name), delim(envDir, name)...))
		case "append":
			p.Vars.Set(name, suffix(p.Vars.Get(name), delim(envDir, name)...)+v)
		case "override":
			p.Vars.Set(name, v)
		case "default":
			if p.Vars.Get(name) != "" {
				return nil
			}
			p.Vars.Set(name, v)
		case "":
			p.Vars.Set(name, v+prefix(p.Vars.Get(name), delim(envDir, name, os.PathListSeparator)...))
		}
		return nil
	})
}

func (p *Env) WithPlatform(platformDir string) (out []string, err error) {
	vars := NewVars(p.Vars.vals, p.Vars.ignoreCase)

	if err := eachEnvFile(filepath.Join(platformDir, "env"), func(k, v string) error {
		if p.isRootEnv(k) {
			vars.Set(k, v+prefix(vars.Get(k), os.PathListSeparator))
			return nil
		}
		vars.Set(k, v)
		return nil
	}); err != nil {
		return nil, err
	}
	return vars.List(), nil
}

func (p *Env) List() []string {
	return p.Vars.List()
}

// Get returns the value for the given key
func (p *Env) Get(k string) string {
	return p.Vars.Get(k)
}

func prefix(s string, prefix ...byte) string {
	if s == "" {
		return ""
	}
	return string(prefix) + s
}

func suffix(s string, suffix ...byte) string {
	if s == "" {
		return ""
	}
	return s + string(suffix)
}

func delim(dir, name string, def ...byte) []byte {
	value, err := ioutil.ReadFile(filepath.Join(dir, name+".delim"))
	if err != nil {
		return def
	}
	return value
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

func (p *Env) isRootEnv(name string) bool {
	for _, m := range p.RootDirMap {
		for _, k := range m {
			if k == name {
				return true
			}
		}
	}
	return false
}
