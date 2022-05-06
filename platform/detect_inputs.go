package platform

// DetectInputs holds the values of command-line flags and args.
// Fields are the cumulative total of inputs across all supported platform APIs.
type DetectInputs struct {
	AppDir        string
	BuildpacksDir string
	ExtensionsDir string
	GroupPath     string
	LayersDir     string
	OrderPath     string
	PlanPath      string
	PlatformDir   string
}

// ResolveDetect accepts a DetectInputs and returns a new DetectInputs with default values filled in,
// or an error if the provided inputs are not valid.
func (r *InputsResolver) ResolveDetect(inputs DetectInputs) (DetectInputs, error) {
	resolvedInputs := inputs

	if err := r.fillDetectDefaults(&resolvedInputs); err != nil {
		return DetectInputs{}, err
	}

	if err := r.validateDetect(resolvedInputs); err != nil {
		return DetectInputs{}, err
	}
	return resolvedInputs, nil
}

func (r *InputsResolver) fillDetectDefaults(inputs *DetectInputs) error {
	if inputs.OrderPath == PlaceholderOrderPath {
		inputs.OrderPath = defaultPath(PlaceholderOrderPath, inputs.LayersDir, r.platformAPI)
	}
	if inputs.GroupPath == PlaceholderGroupPath {
		inputs.GroupPath = defaultPath(PlaceholderGroupPath, inputs.LayersDir, r.platformAPI)
	}
	if inputs.PlanPath == PlaceholderPlanPath {
		inputs.PlanPath = defaultPath(PlaceholderPlanPath, inputs.LayersDir, r.platformAPI)
	}
	return nil
}

func (r *InputsResolver) validateDetect(inputs DetectInputs) error {
	return nil
}
