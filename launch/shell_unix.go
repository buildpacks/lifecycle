// +build linux darwin

package launch

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/api"
)

var (
	bashCommandWithScript = `exec bash -c "$@"` // for processes w/o arguments
	profileGlob           = "*"
	appProfile            = ".profile"
)

func (l *Launcher) execWithShell(self string, process Process, profiles []string) error {
	launcher := ""
	for _, profile := range profiles {
		launcher += fmt.Sprintf("source \"%s\"\n", profile)
	}
	bashCommand, err := l.bashCommand(process)
	if err != nil {
		return errors.Wrap(err, "determine bash command")
	}
	launcher += bashCommand
	if err := l.Exec("/bin/bash", append([]string{
		"bash", "-c",
		launcher, self, process.Command,
	}, process.Args...), l.Env.List()); err != nil {
		return errors.Wrap(err, "bash exec")
	}
	return nil
}

func (l *Launcher) bashCommand(process Process) (string, error) {
	if len(process.Args) == 0 {
		return bashCommandWithScript, nil
	}
	if process.BuildpackID == "" {
		return bashCommandWithTokens(len(process.Args) + 1), nil
	}
	for _, bp := range l.Buildpacks {
		if bp.ID != process.BuildpackID {
			continue
		}
		bpAPI, err := api.NewVersion(bp.API)
		if err != nil {
			return "", fmt.Errorf("failed to parse api '%s' of buildpack '%s'", bp.API, bp.ID)
		}
		if isLegacyProcess(bpAPI) {
			return bashCommandWithScript, nil
		}
		return bashCommandWithTokens(len(process.Args) + 1), nil
	}
	return "", fmt.Errorf("process type '%s' provided by unknown buildpack '%s'", process.Type, process.BuildpackID)
}

func isLegacyProcess(bpAPI *api.Version) bool {
	return bpAPI.Compare(api.MustParse("0.4")) == -1
}

// bashCommandWithTokens returns a bash script that should be executed with nTokens number of bash arguments
//  Each argument to bash is evaluated by the shell before becoming a token in the resulting script
//  Example:
//    Given nTokens=2 the returned script will contain `"$(eval echo \"$0\")" "$(eval echo \"$1\")"`
//      and should be evaluated with  `bash -c '"$(eval echo \"$0\")" "$(eval echo \"$1\")"' <command> <arg>'
//    Token evaluation example:
//      "$(eval echo \"$0\"`)" //  given $0='$FOO' and $FOO='arg with spaces" && quotes'
//      -> "$(eval echo \"'$FOO'\")"
//      -> "$(echo \"'arg with spaces" && quotes'\")"
//      -> "arg with spaces\" && quotes" // this is an evaluated and properly quoted token
func bashCommandWithTokens(nTokens int) string {
	commandScript := `"$(eval echo \"$0\")"`
	for i := 1; i < nTokens; i++ {
		commandScript += fmt.Sprintf(` "$(eval echo \"$%d\")"`, i)
	}
	return fmt.Sprintf(`exec bash -c '%s' "${@:1}"`, commandScript)
}
