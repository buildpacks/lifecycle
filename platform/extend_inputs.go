package platform

// ExtendInputs holds the values of command-line flags and args.
// Fields are the cumulative total of inputs across all supported platform APIs.
type ExtendInputs struct {
	AppDir       string
	GeneratedDir string
	GroupPath    string
	ImageRef     string
	UID, GID     int

	BuildInputs BuildInputs
}

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
		inputs.GeneratedDir = defaultPath(PlaceholderGeneratedDir, inputs.BuildInputs.LayersDir, r.platformAPI)
	}
	if inputs.GroupPath == PlaceholderGroupPath {
		inputs.GroupPath = defaultPath(PlaceholderGroupPath, inputs.BuildInputs.LayersDir, r.platformAPI)
	}
}

func (r *InputsResolver) resolveExtendDirPaths(inputs *ExtendInputs) error {
	var err error
	if inputs.AppDir, err = absoluteIfNotEmpty(inputs.AppDir); err != nil {
		return err
	}
	if inputs.GeneratedDir, err = absoluteIfNotEmpty(inputs.GeneratedDir); err != nil {
		return err
	}
	return nil
}
