package launch

import (
	"github.com/pkg/errors"
)

var (
	profileGlob = "*.bat"
	appProfile  = ".profile.bat"
)

func (l *Launcher) execWithShell(self string, process Process, profiles []string) error {
	var commandTokens []string
	for _, profile := range profiles {
		commandTokens = append(commandTokens, "call", profile, "&&")
	}
	commandTokens = append(commandTokens, process.Command)
	commandTokens = append(commandTokens, process.Args...)
	if err := l.Exec("cmd",
		append([]string{"cmd", "/q", "/c"}, commandTokens...), l.Env.List(),
	); err != nil {
		return errors.Wrap(err, "cmd execute")
	}
	return nil
}
