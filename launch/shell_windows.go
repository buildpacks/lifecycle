package launch

import (
	"fmt"

	"github.com/pkg/errors"
)

var (
	profileGlob = "*.bat"
	appProfile  = ".profile.bat"
)

func (l *Launcher) execWithShell(self string, process Process, profiles []string) error {
	var launcher string
	for _, profile := range profiles {
		launcher += fmt.Sprintf("call %s && ", profile)
	}
	if err := l.Exec("cmd",
		append(append([]string{"cmd", "/q", "/s", "/c"}, launcher, process.Command), process.Args...), l.Env.List(),
	); err != nil {
		return errors.Wrap(err, "cmd execute")
	}
	return nil
}
