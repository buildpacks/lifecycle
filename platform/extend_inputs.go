package platform

// ExtendInputs holds the values of command-line flags and args.
// Fields are the cumulative total of inputs across all supported platform APIs.
type ExtendInputs struct { // TODO: use in cmd/lifecycle/builder.go
	AppDir       string // TODO: use in extender
	GeneratedDir string
	GroupPath    string // TODO: will this clobber the group path in BuildInputs?
	ImageRef     string
	UID, GID     int
	// TODO: add more here

	BuildInputs
}

// TODO: see if some logic from cmd/lifecycle/cli/flags.go can move to this package

// ResolveExtend accepts a ExtendInputs and returns a new ExtendInputs with default values filled in,
// or an error if the provided inputs are not valid.
func (r *InputsResolver) ResolveExtend(inputs ExtendInputs) (ExtendInputs, error) {
	resolvedInputs := inputs

	r.fillExtendDefaultPaths(&resolvedInputs)

	if err := r.resolveExtendDirPaths(&resolvedInputs); err != nil {
		return ExtendInputs{}, err
	}
	return resolvedInputs, nil
}

func (r *InputsResolver) fillExtendDefaultPaths(inputs *ExtendInputs) {
	if inputs.GeneratedDir == PlaceholderGeneratedDir {
		inputs.GeneratedDir = defaultPath(PlaceholderGeneratedDir, inputs.LayersDir, r.platformAPI)
	}
	if inputs.GroupPath == PlaceholderGroupPath {
		inputs.GroupPath = defaultPath(PlaceholderGroupPath, inputs.LayersDir, r.platformAPI)
	}
	if inputs.PlanPath == PlaceholderPlanPath {
		inputs.PlanPath = defaultPath(PlaceholderPlanPath, inputs.LayersDir, r.platformAPI)
	}
}

func (r *InputsResolver) resolveExtendDirPaths(inputs *ExtendInputs) error {
	var err error
	if inputs.AppDir, err = absoluteIfNotEmpty(inputs.AppDir); err != nil {
		return err
	}
	if inputs.BuildpacksDir, err = absoluteIfNotEmpty(inputs.BuildpacksDir); err != nil {
		return err
	}
	if inputs.GeneratedDir, err = absoluteIfNotEmpty(inputs.GeneratedDir); err != nil {
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
