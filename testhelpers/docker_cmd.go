package testhelpers

type DockerCmd struct {
	flags []string
	args  []string
}

type DockerCmdOp func(*DockerCmd)

func WithFlags(flags ...string) DockerCmdOp {
	return func(cmd *DockerCmd) {
		cmd.flags = append(cmd.flags, flags...)
	}
}

func WithArgs(args ...string) DockerCmdOp {
	return func(cmd *DockerCmd) {
		cmd.args = append(cmd.args, args...)
	}
}

func WithBash(args ...string) DockerCmdOp {
	return func(cmd *DockerCmd) {
		cmd.args = append([]string{"/bin/bash", "-c"}, args...)
	}
}

func formatArgs(args []string, ops ...DockerCmdOp) []string {
	cmd := DockerCmd{}

	for _, op := range ops {
		op(&cmd)
	}

	args = append(cmd.flags, args...) // prepend flags
	return append(args, cmd.args...)  // append args
}
