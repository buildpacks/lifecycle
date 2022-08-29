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
	"github.com/buildpacks/lifecycle/internal/encoding"
	"github.com/buildpacks/lifecycle/internal/fsutil"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/layers"
	"github.com/buildpacks/lifecycle/log"
)

const (
	EnvLayersDir  = "CNB_LAYERS_DIR"
	EnvBpPlanPath = "CNB_BP_PLAN_PATH"

	EnvOutputDir = "CNB_OUTPUT_DIR"
)

type BuildEnv interface {
	AddRootDir(baseDir string) error
	AddEnvDir(envDir string, defaultAction env.ActionType) error
	WithOverrides(platformDir string, buildConfigDir string) ([]string, error)
	List() []string
}

type BuildConfig struct {
	AppDir          string
	OutputParentDir string
	PlatformDir     string
	BuildConfigDir  string
	Out             io.Writer
	Err             io.Writer
	Logger          log.Logger
}

type BuildResult struct {
	BOMFiles    []BOMFile
	BuildBOM    []BOMEntry
	Dockerfiles []Dockerfile
	Labels      []Label
	LaunchBOM   []BOMEntry
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

func (d *Descriptor) Build(plan Plan, config BuildConfig, buildEnv BuildEnv) (BuildResult, error) {
	if api.MustParse(d.API).Equal(api.MustParse("0.2")) {
		config.Logger.Debug("Updating plan entries")
		for i := range plan.Entries {
			plan.Entries[i].convertMetadataToVersion()
		}
	}

	config.Logger.Debug("Creating plan directory")
	planDir, err := ioutil.TempDir("", launch.EscapeID(d.Info().ID)+"-")
	if err != nil {
		return BuildResult{}, err
	}
	defer os.RemoveAll(planDir)

	config.Logger.Debug("Preparing paths")
	moduleOutputDir, planPath, err := preparePaths(d.Info().ID, plan, config.OutputParentDir, planDir)
	if err != nil {
		return BuildResult{}, err
	}

	config.Logger.Debug("Running build command")
	_, err = os.Stat(filepath.Join(d.Dir, "bin", "generate"))
	if d.IsExtension() && os.IsNotExist(err) {
		// treat extension root directory as pre-populated output directory
		return d.readOutputFilesExt(filepath.Join(d.Dir, "generate"), plan)
	} else if err = d.runCmd(moduleOutputDir, planPath, config, buildEnv); err != nil {
		return BuildResult{}, err
	}

	config.Logger.Debug("Processing layers")
	createdLayers, err := d.processLayers(moduleOutputDir, config.Logger)
	if err != nil {
		return BuildResult{}, err
	}

	config.Logger.Debug("Updating environment")
	if err := d.setupEnv(createdLayers, buildEnv); err != nil {
		return BuildResult{}, err
	}

	config.Logger.Debug("Reading output files")
	if d.IsExtension() {
		return d.readOutputFilesExt(moduleOutputDir, plan)
	}
	return d.readOutputFilesBp(moduleOutputDir, planPath, plan, createdLayers, config.Logger)
}

func renameLayerDirIfNeeded(layerMetadataFile LayerMetadataFile, layerDir string) error {
	// rename <layers>/<layer> to <layers>/<layer>.ignore if buildpack API >= 0.6 and all of the types flags are set to false
	if !layerMetadataFile.Launch && !layerMetadataFile.Cache && !layerMetadataFile.Build {
		if err := fsutil.RenameWithWindowsFallback(layerDir, layerDir+".ignore"); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func (d *Descriptor) processLayers(layersDir string, logger log.Logger) (map[string]LayerMetadataFile, error) {
	if d.IsExtension() {
		return map[string]LayerMetadataFile{}, nil
	}
	if api.MustParse(d.API).LessThan("0.6") {
		return eachLayer(layersDir, d.API, func(path, buildpackAPI string) (LayerMetadataFile, error) {
			layerMetadataFile, msg, err := DecodeLayerMetadataFile(path+".toml", buildpackAPI)
			if err != nil {
				return LayerMetadataFile{}, err
			}
			if msg != "" {
				logger.Warn(msg)
			}
			return layerMetadataFile, nil
		})
	}
	return eachLayer(layersDir, d.API, func(path, buildpackAPI string) (LayerMetadataFile, error) {
		layerMetadataFile, msg, err := DecodeLayerMetadataFile(path+".toml", buildpackAPI)
		if err != nil {
			return LayerMetadataFile{}, err
		}
		if msg != "" {
			return LayerMetadataFile{}, errors.New(msg)
		}
		if err := renameLayerDirIfNeeded(layerMetadataFile, path); err != nil {
			return LayerMetadataFile{}, err
		}
		return layerMetadataFile, nil
	})
}

func preparePaths(moduleID string, plan Plan, outputParentDir, parentPlanDir string) (string, string, error) {
	moduleDirName := launch.EscapeID(moduleID) // TODO: this logic should eventually move to the platform package
	moduleOutputDir := filepath.Join(outputParentDir, moduleDirName)
	childPlanDir := filepath.Join(parentPlanDir, moduleDirName)
	if err := os.MkdirAll(moduleOutputDir, 0777); err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(childPlanDir, 0777); err != nil {
		return "", "", err
	}
	planPath := filepath.Join(childPlanDir, "plan.toml")
	if err := encoding.WriteTOML(planPath, plan); err != nil {
		return "", "", err
	}

	return moduleOutputDir, planPath, nil
}

func (d *Descriptor) runCmd(moduleOutputDir, planPath string, config BuildConfig, buildEnv BuildEnv) error {
	cmdName := "build"
	if d.IsExtension() {
		cmdName = "generate"
	}
	cmd := exec.Command(
		filepath.Join(d.Dir, "bin", cmdName),
		moduleOutputDir,
		config.PlatformDir,
		planPath,
	) // #nosec G204
	cmd.Dir = config.AppDir
	cmd.Stdout = config.Out
	cmd.Stderr = config.Err

	var err error
	platformDir := config.PlatformDir
	if d.Buildpack.ClearEnv {
		platformDir = ""
	}
	cmd.Env, err = buildEnv.WithOverrides(platformDir, config.BuildConfigDir)
	if err != nil {
		return err
	}
	cmd.Env = append(cmd.Env, EnvBuildpackDir+"="+d.Dir)
	if api.MustParse(d.API).AtLeast("0.8") {
		cmd.Env = append(cmd.Env,
			EnvPlatformDir+"="+config.PlatformDir,
			EnvBpPlanPath+"="+planPath,
		)
		if d.IsExtension() {
			cmd.Env = append(cmd.Env, EnvOutputDir+"="+moduleOutputDir)
		} else {
			cmd.Env = append(cmd.Env, EnvLayersDir+"="+moduleOutputDir)
		}
	}

	if err := cmd.Run(); err != nil {
		return NewError(err, ErrTypeBuildpack)
	}
	return nil
}

func (d *Descriptor) setupEnv(createdLayers map[string]LayerMetadataFile, buildEnv BuildEnv) error {
	bpAPI := api.MustParse(d.API)
	for path, layerMetadataFile := range createdLayers {
		if !layerMetadataFile.Build {
			continue
		}
		if err := buildEnv.AddRootDir(path); err != nil {
			return err
		}
		if err := buildEnv.AddEnvDir(filepath.Join(path, "env"), env.DefaultActionType(bpAPI)); err != nil {
			return err
		}
		if err := buildEnv.AddEnvDir(filepath.Join(path, "env.build"), env.DefaultActionType(bpAPI)); err != nil {
			return err
		}
	}
	return nil
}

func eachLayer(bpLayersDir, buildpackAPI string, fn func(path, api string) (LayerMetadataFile, error)) (map[string]LayerMetadataFile, error) {
	files, err := ioutil.ReadDir(bpLayersDir)
	if os.IsNotExist(err) {
		return map[string]LayerMetadataFile{}, nil
	} else if err != nil {
		return map[string]LayerMetadataFile{}, err
	}
	bpLayers := map[string]LayerMetadataFile{}
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".toml") {
			continue
		}
		path := filepath.Join(bpLayersDir, strings.TrimSuffix(f.Name(), ".toml"))
		layerMetadataFile, err := fn(path, buildpackAPI)
		if err != nil {
			return map[string]LayerMetadataFile{}, err
		}
		bpLayers[path] = layerMetadataFile
	}
	return bpLayers, nil
}

func (d *Descriptor) readOutputFilesBp(bpLayersDir, bpPlanPath string, bpPlanIn Plan, bpLayers map[string]LayerMetadataFile, logger log.Logger) (BuildResult, error) {
	br := BuildResult{}
	bpFromBpInfo := GroupElement{ID: d.Info().ID, Version: d.Buildpack.Version}

	// setup launch.toml
	var launchTOML LaunchTOML
	launchPath := filepath.Join(bpLayersDir, "launch.toml")

	bomValidator := NewBOMValidator(d.API, bpLayersDir, logger)

	var err error
	if api.MustParse(d.API).LessThan("0.5") {
		// read buildpack plan
		var bpPlanOut Plan
		if _, err := toml.DecodeFile(bpPlanPath, &bpPlanOut); err != nil {
			return BuildResult{}, err
		}

		// set BOM and MetRequires
		br.LaunchBOM, err = bomValidator.ValidateBOM(bpFromBpInfo, bpPlanOut.toBOM())
		if err != nil {
			return BuildResult{}, err
		}
		br.MetRequires = names(bpPlanOut.Entries)

		// set BOM files
		br.BOMFiles, err = d.processSBOMFiles(bpLayersDir, bpFromBpInfo, bpLayers, logger)
		if err != nil {
			return BuildResult{}, err
		}

		// read launch.toml, return if not exists
		if err := DecodeLaunchTOML(launchPath, d.API, &launchTOML); os.IsNotExist(err) {
			return br, nil
		} else if err != nil {
			return BuildResult{}, err
		}
	} else {
		// read build.toml
		var buildTOML BuildTOML
		buildPath := filepath.Join(bpLayersDir, "build.toml")
		if _, err := toml.DecodeFile(buildPath, &buildTOML); err != nil && !os.IsNotExist(err) {
			return BuildResult{}, err
		}
		if _, err := bomValidator.ValidateBOM(bpFromBpInfo, buildTOML.BOM); err != nil {
			return BuildResult{}, err
		}
		br.BuildBOM, err = bomValidator.ValidateBOM(bpFromBpInfo, buildTOML.BOM)
		if err != nil {
			return BuildResult{}, err
		}

		// set MetRequires
		if err := validateUnmet(buildTOML.Unmet, bpPlanIn); err != nil {
			return BuildResult{}, err
		}
		br.MetRequires = names(bpPlanIn.filter(buildTOML.Unmet).Entries)

		// set BOM files
		br.BOMFiles, err = d.processSBOMFiles(bpLayersDir, bpFromBpInfo, bpLayers, logger)
		if err != nil {
			return BuildResult{}, err
		}

		// read launch.toml, return if not exists
		if err := DecodeLaunchTOML(launchPath, d.API, &launchTOML); os.IsNotExist(err) {
			return br, nil
		} else if err != nil {
			return BuildResult{}, err
		}

		// set BOM
		br.LaunchBOM, err = bomValidator.ValidateBOM(bpFromBpInfo, launchTOML.BOM)
		if err != nil {
			return BuildResult{}, err
		}
	}

	if err := overrideDefaultForOldBuildpacks(launchTOML.Processes, d.API, logger); err != nil {
		return BuildResult{}, err
	}

	if err := validateNoMultipleDefaults(launchTOML.Processes); err != nil {
		return BuildResult{}, err
	}

	// set data from launch.toml
	br.Labels = append([]Label{}, launchTOML.Labels...)
	for i := range launchTOML.Processes {
		if api.MustParse(d.API).LessThan("0.8") {
			if launchTOML.Processes[i].WorkingDirectory != "" {
				logger.Warn(fmt.Sprintf("Warning: process working directory isn't supported in this buildpack api version. Ignoring working directory for process '%s'", launchTOML.Processes[i].Type))
				launchTOML.Processes[i].WorkingDirectory = ""
			}
		}
	}
	br.Processes = append([]launch.Process{}, launchTOML.ToLaunchProcessesForBuildpack(d.Info().ID)...)
	br.Slices = append([]layers.Slice{}, launchTOML.Slices...)

	return br, nil
}

func (d *Descriptor) readOutputFilesExt(extOutputDir string, extPlanIn Plan) (BuildResult, error) {
	br := BuildResult{}
	var err error

	// set MetRequires
	br.MetRequires = names(extPlanIn.Entries)

	// set Dockerfiles
	runDockerfile := filepath.Join(extOutputDir, "run.Dockerfile")
	if _, err = os.Stat(runDockerfile); err != nil {
		if os.IsNotExist(err) {
			return br, nil
		}
		return BuildResult{}, err
	}
	br.Dockerfiles = []Dockerfile{{ExtensionID: d.Info().ID, Kind: DockerfileKindRun, Path: runDockerfile}}
	return br, nil
}

func overrideDefaultForOldBuildpacks(processes []ProcessEntry, bpAPI string, logger log.Logger) error {
	if api.MustParse(bpAPI).AtLeast("0.6") {
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
		logger.Warn(fmt.Sprintf("Warning: default processes aren't supported in this buildpack api version. Overriding the default value to false for the following processes: [%s]", strings.Join(replacedDefaults, ", ")))
	}
	return nil
}

func validateNoMultipleDefaults(processes []ProcessEntry) error {
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

func WithBuildpack(bp GroupElement, bom []BOMEntry) []BOMEntry {
	var out []BOMEntry
	for _, entry := range bom {
		entry.Buildpack = bp.NoAPI().NoHomepage()
		out = append(out, entry)
	}
	return out
}
