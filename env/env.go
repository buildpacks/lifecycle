package env

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

type Env struct {
	RootDirMap map[string][]string
	vars       map[string]string
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
			p.vars[key] = newDir + prefix(p.vars[key], os.PathListSeparator)
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
			p.vars[name] = v + prefix(p.vars[name], delim(envDir, name)...)
		case "append":
			p.vars[name] = suffix(p.vars[name], delim(envDir, name)...) + v
		case "override":
			p.vars[name] = v
		case "default":
			if p.vars[name] != "" {
				return nil
			}
			p.vars[name] = v
		case "":
			p.vars[name] = v + prefix(p.vars[name], delim(envDir, name, os.PathListSeparator)...)
		}
		return nil
	})
}

func (p *Env) WithPlatform(platformDir string) (out []string, err error) {
	restore := map[string]*string{}
	defer func() {
		for k, v := range restore {
			var rErr error
			if v == nil {
				delete(p.vars, k)
			} else {
				p.vars[k] = *v
			}
			if err == nil {
				err = rErr
			}
		}
	}()
	if err := eachEnvFile(filepath.Join(platformDir, "env"), func(k, v string) error {
		restore[k] = nil
		if old, ok := p.vars[k]; ok {
			restore[k] = &old
		}
		if p.isRootEnv(k) {
			p.vars[k] = v + prefix(p.vars[k], os.PathListSeparator)
			return nil
		}
		p.vars[k] = v
		return nil
	}); err != nil {
		return nil, err
	}
	return p.List(), nil
}

func (p *Env) List() []string {
	var environ []string
	for k, v := range p.vars {
		environ = append(environ, k+"="+v)
	}
	return environ
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
