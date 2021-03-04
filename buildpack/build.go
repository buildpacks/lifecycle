package buildpack

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/env"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/layers"
)

type BuildEnv interface {
	AddRootDir(baseDir string) error
	AddEnvDir(envDir string, defaultAction env.ActionType) error
	WithPlatform(platformDir string) ([]string, error)
	List() []string
}

type BuildConfig struct {
	Env         BuildEnv
	AppDir      string
	PlatformDir string
	LayersDir   string
	Out         io.Writer
	Err         io.Writer
}

type BuildResult struct {
	BOM         []BOMEntry
	Labels      []Label
	MetRequires []string
	Processes   []launch.Process
	Slices      []layers.Slice
}

func (bom *BOMEntry) ConvertMetadataToVersion() {
	if version, ok := bom.Metadata["version"]; ok {
		metadataVersion := fmt.Sprintf("%v", version)
		bom.Version = metadataVersion
	}
}

func (bom *BOMEntry) convertVersionToMetadata() {
	if bom.Version != "" {
		if bom.Metadata == nil {
			bom.Metadata = make(map[string]interface{})
		}
		bom.Metadata["version"] = bom.Version
		bom.Version = ""
	}
}

func (b *Descriptor) Build(bpPlan Plan, config BuildConfig) (BuildResult, error) {
	if api.MustParse(b.API).Equal(api.MustParse("0.2")) {
		for i := range bpPlan.Entries {
			bpPlan.Entries[i].convertMetadataToVersion()
		}
	}

	planDir, err := ioutil.TempDir("", launch.EscapeID(b.Buildpack.ID)+"-")
	if err != nil {
		return BuildResult{}, err
	}
	defer os.RemoveAll(planDir)

	bpLayersDir, bpPlanPath, err := preparePaths(b.Buildpack.ID, bpPlan, config.LayersDir, planDir)
	if err != nil {
		return BuildResult{}, err
	}

	if err := b.runBuildCmd(bpLayersDir, bpPlanPath, config); err != nil {
		return BuildResult{}, err
	}

	if err := b.checkTypesFormat(bpLayersDir, config.Out); err != nil {
		return BuildResult{}, err
	}

	if err := b.setupEnv(config.Env, bpLayersDir); err != nil {
		return BuildResult{}, err
	}

	return b.readOutputFiles(bpLayersDir, bpPlanPath, bpPlan, config.Out)
}

func renameLayerDirIfNeeded(layerMetadataFile LayerMetadataFile, layerDir string) error {
	// rename <layers>/<layer> to <layers>/<layer>.ignore if buildpack API >= 0.6 and all of the types flags are set to false
	if !layerMetadataFile.Launch && !layerMetadataFile.Cache && !layerMetadataFile.Build {
		if err := os.Rename(layerDir, layerDir+".ignore"); err != nil {
			return err
		}
	}
	return nil
}

func (b *Descriptor) checkTypesFormat(layersDir string, out io.Writer) error {
	if api.MustParse(b.API).Compare(api.MustParse("0.6")) < 0 {
		return eachDir(layersDir, b.API, func(path, buildpackAPI string) error {
			_, rightFormat, err := DecodeLayerMetadataFile(path+".toml", buildpackAPI)
			if err != nil {
				return err
			}
			if !rightFormat {
				warning := "Warning: types table isn't supported in this buildpack api version. The launch, build and cache flags should be in the top level. Ignoring the values in the types table."
				if _, err = out.Write([]byte(warning)); err != nil {
					return err
				}
			}
			return nil
		})
	}
	return eachDir(layersDir, b.API, func(path, buildpackAPI string) error {
		layerMetadataFile, rightFormat, err := DecodeLayerMetadataFile(path+".toml", buildpackAPI)
		if err != nil {
			return err
		}
		if !rightFormat {
			return fmt.Errorf("the launch, cache and build flags should be in the types table of %s.toml", path)
		}
		if err := renameLayerDirIfNeeded(layerMetadataFile, path); err != nil {
			return err
		}
		return nil
	})
}

func preparePaths(bpID string, bpPlan Plan, layersDir, planDir string) (string, string, error) {
	bpDirName := launch.EscapeID(bpID)
	bpLayersDir := filepath.Join(layersDir, bpDirName)
	bpPlanDir := filepath.Join(planDir, bpDirName)
	if err := os.MkdirAll(bpLayersDir, 0777); err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(bpPlanDir, 0777); err != nil {
		return "", "", err
	}
	bpPlanPath := filepath.Join(bpPlanDir, "plan.toml")
	if err := WriteTOML(bpPlanPath, bpPlan); err != nil {
		return "", "", err
	}

	return bpLayersDir, bpPlanPath, nil
}

func WriteTOML(path string, data interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0777); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(data)
}

func (b *Descriptor) runBuildCmd(bpLayersDir, bpPlanPath string, config BuildConfig) error {
	cmd := exec.Command(
		filepath.Join(b.Dir, "bin", "build"),
		bpLayersDir,
		config.PlatformDir,
		bpPlanPath,
	)
	cmd.Dir = config.AppDir
	cmd.Stdout = config.Out
	cmd.Stderr = config.Err

	var err error
	if b.Buildpack.ClearEnv {
		cmd.Env = config.Env.List()
	} else {
		cmd.Env, err = config.Env.WithPlatform(config.PlatformDir)
		if err != nil {
			return err
		}
	}
	cmd.Env = append(cmd.Env, EnvBuildpackDir+"="+b.Dir)

	if err := cmd.Run(); err != nil {
		return NewLifecycleError(err, ErrTypeBuildpack)
	}
	return nil
}

func (b *Descriptor) setupEnv(buildEnv BuildEnv, layersDir string) error {
	if err := eachDir(layersDir, b.API, func(path, buildpackAPI string) error {
		if !isBuild(path+".toml", buildpackAPI) {
			return nil
		}
		return buildEnv.AddRootDir(path)
	}); err != nil {
		return err
	}

	return eachDir(layersDir, b.API, func(path, buildpackAPI string) error {
		if !isBuild(path+".toml", buildpackAPI) {
			return nil
		}
		bpAPI := api.MustParse(b.API)
		if err := buildEnv.AddEnvDir(filepath.Join(path, "env"), env.DefaultActionType(bpAPI)); err != nil {
			return err
		}
		return buildEnv.AddEnvDir(filepath.Join(path, "env.build"), env.DefaultActionType(bpAPI))
	})
}

func eachDir(dir, buildpackAPI string, fn func(path, api string) error) error {
	files, err := ioutil.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	for _, f := range files {
		if !f.IsDir() {
			continue
		}
		if err := fn(filepath.Join(dir, f.Name()), buildpackAPI); err != nil {
			return err
		}
	}
	return nil
}

func (b *Descriptor) readOutputFiles(bpLayersDir, bpPlanPath string, bpPlanIn Plan, out io.Writer) (BuildResult, error) {
	br := BuildResult{}
	bpFromBpInfo := GroupBuildpack{ID: b.Buildpack.ID, Version: b.Buildpack.Version}

	// setup launch.toml
	var launchTOML LaunchTOML
	launchPath := filepath.Join(bpLayersDir, "launch.toml")

	if api.MustParse(b.API).Compare(api.MustParse("0.5")) < 0 { // buildpack API <= 0.4
		// read buildpack plan
		var bpPlanOut Plan
		if _, err := toml.DecodeFile(bpPlanPath, &bpPlanOut); err != nil {
			return BuildResult{}, err
		}

		// set BOM and MetRequires
		if err := validateBOM(bpPlanOut.toBOM(), b.API); err != nil {
			return BuildResult{}, err
		}
		br.BOM = WithBuildpack(bpFromBpInfo, bpPlanOut.toBOM())
		for i := range br.BOM {
			br.BOM[i].convertVersionToMetadata()
		}
		br.MetRequires = names(bpPlanOut.Entries)

		// read launch.toml, return if not exists
		if _, err := toml.DecodeFile(launchPath, &launchTOML); os.IsNotExist(err) {
			return br, nil
		} else if err != nil {
			return BuildResult{}, err
		}
	} else {
		// read build.toml
		var bpBuild BuildTOML
		buildPath := filepath.Join(bpLayersDir, "build.toml")
		if _, err := toml.DecodeFile(buildPath, &bpBuild); err != nil && !os.IsNotExist(err) {
			return BuildResult{}, err
		}
		if err := validateBOM(bpBuild.BOM, b.API); err != nil {
			return BuildResult{}, err
		}

		// set MetRequires
		if err := validateUnmet(bpBuild.Unmet, bpPlanIn); err != nil {
			return BuildResult{}, err
		}
		br.MetRequires = names(bpPlanIn.filter(bpBuild.Unmet).Entries)

		// read launch.toml, return if not exists
		if _, err := toml.DecodeFile(launchPath, &launchTOML); os.IsNotExist(err) {
			return br, nil
		} else if err != nil {
			return BuildResult{}, err
		}

		// set BOM
		if err := validateBOM(launchTOML.BOM, b.API); err != nil {
			return BuildResult{}, err
		}
		br.BOM = WithBuildpack(bpFromBpInfo, launchTOML.BOM)
	}

	if err := overrideDefaultForOldBuildpacks(launchTOML.Processes, b.API, out); err != nil {
		return BuildResult{}, err
	}

	if err := validateNoMultipleDefaults(launchTOML.Processes); err != nil {
		return BuildResult{}, err
	}

	// set data from launch.toml
	br.Labels = append([]Label{}, launchTOML.Labels...)
	for i := range launchTOML.Processes {
		launchTOML.Processes[i].BuildpackID = b.Buildpack.ID
	}
	br.Processes = append([]launch.Process{}, launchTOML.Processes...)
	br.Slices = append([]layers.Slice{}, launchTOML.Slices...)

	return br, nil
}

func overrideDefaultForOldBuildpacks(processes []launch.Process, bpAPI string, out io.Writer) error {
	if api.MustParse(bpAPI).Compare(api.MustParse("0.6")) >= 0 {
		return nil
	}
	replacedDefaults := []string{}
	for i := range processes {
		if processes[i].Default {
			replacedDefaults = append(replacedDefaults, processes[i].Type)
		}
		processes[i].Default = false
	}
	if len(replacedDefaults) > 0 {
		warning := fmt.Sprintf("Warning: default processes aren't supported in this buildpack api version. Overriding the default value to false for the following processes: [%s]", strings.Join(replacedDefaults, ", "))
		if _, err := out.Write([]byte(warning)); err != nil {
			return err
		}
	}
	return nil
}

func validateNoMultipleDefaults(processes []launch.Process) error {
	defaultType := ""
	for _, process := range processes {
		if process.Default && defaultType != "" {
			return fmt.Errorf("multiple default process types aren't allowed")
		}
		if process.Default {
			defaultType = process.Type
		}
	}
	return nil
}

func validateBOM(bom []BOMEntry, bpAPI string) error {
	if api.MustParse(bpAPI).Compare(api.MustParse("0.5")) < 0 {
		for _, entry := range bom {
			if version, ok := entry.Metadata["version"]; ok {
				metadataVersion := fmt.Sprintf("%v", version)
				if entry.Version != "" && entry.Version != metadataVersion {
					return errors.New("top level version does not match metadata version")
				}
			}
		}
	} else {
		for _, entry := range bom {
			if entry.Version != "" {
				return fmt.Errorf("bom entry '%s' has a top level version which is not allowed. The buildpack should instead set metadata.version", entry.Name)
			}
		}
	}
	return nil
}

func validateUnmet(unmet []Unmet, bpPlan Plan) error {
	for _, unmet := range unmet {
		if unmet.Name == "" {
			return errors.New("unmet.name is required")
		}
		found := false
		for _, req := range bpPlan.Entries {
			if unmet.Name == req.Name {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unmet.name '%s' must match a requested dependency", unmet.Name)
		}
	}
	return nil
}

func names(requires []Require) []string {
	var out []string
	for _, req := range requires {
		out = append(out, req.Name)
	}
	return out
}

func WithBuildpack(bp GroupBuildpack, bom []BOMEntry) []BOMEntry {
	var out []BOMEntry
	for _, entry := range bom {
		entry.Buildpack = bp.NoAPI().NoHomepage()
		out = append(out, entry)
	}
	return out
}
