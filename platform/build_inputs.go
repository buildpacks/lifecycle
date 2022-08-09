package platform

// BuildInputs holds the values of command-line flags and args.
// Fields are the cumulative total of inputs across all supported platform APIs.
type BuildInputs struct { // TODO: use in cmd/lifecycle/builder.go
	AppDir        string
	BuildpacksDir string
	GroupPath     string
	LayersDir     string
	PlanPath      string
	PlatformDir   string
}

// TODO: see if some logic from cmd/lifecycle/cli/flags.go can move to this package

// ResolveBuild accepts a BuildInputs and returns a new BuildInputs with default values filled in,
// or an error if the provided inputs are not valid.
func (r *InputsResolver) ResolveBuild(inputs BuildInputs) (BuildInputs, error) {
	resolvedInputs := inputs

	r.fillBuildDefaultFilePaths(&resolvedInputs)

	if err := r.resolveBuildDirPaths(&resolvedInputs); err != nil {
		return BuildInputs{}, err
	}
	return resolvedInputs, nil
}

func (r *InputsResolver) fillBuildDefaultFilePaths(inputs *BuildInputs) {
	if inputs.GroupPath == PlaceholderGroupPath {
		inputs.GroupPath = defaultPath(PlaceholderGroupPath, inputs.LayersDir, r.platformAPI)
	}
	if inputs.PlanPath == PlaceholderPlanPath {
		inputs.PlanPath = defaultPath(PlaceholderPlanPath, inputs.LayersDir, r.platformAPI)
	}
}

func (r *InputsResolver) resolveBuildDirPaths(inputs *BuildInputs) error {
	var err error
	if inputs.AppDir, err = absoluteIfNotEmpty(inputs.AppDir); err != nil {
		return err
	}
	if inputs.BuildpacksDir, err = absoluteIfNotEmpty(inputs.BuildpacksDir); err != nil {
		return err
	}
	if inputs.LayersDir, err = absoluteIfNotEmpty(inputs.LayersDir); err != nil {
		return err
	}
	if inputs.PlatformDir, err = absoluteIfNotEmpty(inputs.PlatformDir); err != nil {
		return err
	}
	return nil
}
