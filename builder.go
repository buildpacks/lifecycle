package lifecycle

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/BurntSushi/toml"
)

type Builder struct {
	PlatformDir string
	CacheDir    string
	LaunchDir   string
	AppDir      string
	Env         BuildEnv
	Buildpacks  []*Buildpack
	Plan        Plan
	Out, Err    io.Writer
}

type BuildEnv interface {
	AddRootDir(baseDir string) error
	AddEnvDir(envDir string) error
	List() []string
}

type Process struct {
	Type    string `toml:"type"`
	Command string `toml:"command"`
}

type LaunchTOML struct {
	Processes []Process `toml:"processes"`
}

type Plan map[string]map[string]interface{}

type BuildMetadata struct {
	Processes  []Process `toml:"processes"`
	Buildpacks []string  `toml:"buildpacks"`
	BOM        Plan      `toml:"bom"`
}

func (b *Builder) Build() (*BuildMetadata, error) {
	platformDir, err := filepath.Abs(b.PlatformDir)
	if err != nil {
		return nil, err
	}
	launchDir, err := filepath.Abs(b.LaunchDir)
	if err != nil {
		return nil, err
	}
	cacheDir, err := filepath.Abs(b.CacheDir)
	if err != nil {
		return nil, err
	}
	appDir, err := filepath.Abs(b.AppDir)
	if err != nil {
		return nil, err
	}
	planDir, err := ioutil.TempDir("", "plan.")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(planDir)

	procMap := processMap{}
	plan := copyPlan(b.Plan)
	bom := copyPlan(b.Plan)
	var buildpackIDs []string
	for _, bp := range b.Buildpacks {
		bpDirName := bp.EscapedID()
		bpLaunchDir := filepath.Join(launchDir, bpDirName)
		bpCacheDir := filepath.Join(cacheDir, bpDirName)
		bpPlanDir := filepath.Join(planDir, bpDirName)
		buildpackIDs = append(buildpackIDs, bpDirName)
		if err := os.MkdirAll(bpLaunchDir, 0777); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(bpCacheDir, 0777); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(bpPlanDir, 0777); err != nil {
			return nil, err
		}
		planIn := &bytes.Buffer{}
		if err := toml.NewEncoder(planIn).Encode(plan); err != nil {
			return nil, err
		}
		buildPath, err := filepath.Abs(filepath.Join(bp.Dir, "bin", "build"))
		if err != nil {
			return nil, err
		}
		cmd := exec.Command(buildPath, platformDir, bpPlanDir, bpCacheDir, bpLaunchDir)
		cmd.Env = b.Env.List()
		cmd.Dir = appDir
		cmd.Stdin = planIn
		cmd.Stdout = b.Out
		cmd.Stderr = b.Err
		if err := cmd.Run(); err != nil {
			return nil, err
		}
		if err := setupEnv(b.Env, bpCacheDir); err != nil {
			return nil, err
		}
		if err := consumePlan(bpPlanDir, plan, bom); err != nil {
			return nil, err
		}
		var launch LaunchTOML
		tomlPath := filepath.Join(bpLaunchDir, "launch.toml")
		if _, err := toml.DecodeFile(tomlPath, &launch); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, err
		}
		procMap.add(launch.Processes)
	}

	return &BuildMetadata{
		Processes:  procMap.list(),
		Buildpacks: buildpackIDs,
		BOM:        bom,
	}, nil
}

func setupEnv(env BuildEnv, cacheDir string) error {
	cacheFiles, err := ioutil.ReadDir(cacheDir)
	if err != nil {
		return err
	}
	if err := eachDir(cacheFiles, func(layer os.FileInfo) error {
		return env.AddRootDir(filepath.Join(cacheDir, layer.Name()))
	}); err != nil {
		return err
	}
	return eachDir(cacheFiles, func(layer os.FileInfo) error {
		return env.AddEnvDir(filepath.Join(cacheDir, layer.Name(), "env"))
	})
}

func eachDir(files []os.FileInfo, fn func(os.FileInfo) error) error {
	for _, f := range files {
		if !f.IsDir() {
			continue
		}
		if err := fn(f); err != nil {
			return err
		}
	}
	return nil
}

func consumePlan(planDir string, plan, bom Plan) error {
	files, err := ioutil.ReadDir(planDir)
	if err != nil {
		return err
	}
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		path := filepath.Join(planDir, f.Name())
		var entry map[string]interface{}
		if _, err := toml.DecodeFile(path, &entry); err != nil {
			return err
		}
		delete(plan, f.Name())
		if len(entry) > 0 {
			bom[f.Name()] = entry
		}
	}
	return nil
}

type processMap map[string]Process

func (m processMap) add(l []Process) {
	for _, proc := range l {
		m[proc.Type] = proc
	}
}

func (m processMap) list() []Process {
	var keys []string
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	procs := []Process{}
	for _, key := range keys {
		procs = append(procs, m[key])
	}
	return procs
}

func copyPlan(m Plan) Plan {
	out := Plan{}
	for k, v := range m {
		out[k] = v
	}
	return out
}