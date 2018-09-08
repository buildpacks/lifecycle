package cmd

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
)

const (
	EnvLaunchDir     = "PACK_LAUNCH_DIR"
	EnvBuildpacksDir = "PACK_BUILDPACKS_DIR"

	EnvOrderPath = "PACK_ORDER_PATH"
	EnvGroupPath = "PACK_GROUP_PATH"
	EnvPlanPath  = "PACK_PLAN_PATH"

	EnvRunImage = "PACK_RUN_IMAGE"

	EnvUseDaemon  = "PACK_USE_DAEMON"
	EnvUseHelpers = "PACK_USE_HELPERS"
)

func FlagLaunchDir(dir *string) {
	flag.StringVar(dir, "launch", os.Getenv(EnvLaunchDir), "path to launch directory")
}

func FlagBuildpacksDir(dir *string) {
	flag.StringVar(dir, "buildpacks", os.Getenv(EnvBuildpacksDir), "path to buildpacks directory")
}

func FlagOrderPath(path *string) {
	flag.StringVar(path, "order", os.Getenv(EnvOrderPath), "path to order.toml")
}

func FlagGroupPath(path *string) {
	flag.StringVar(path, "group", os.Getenv(EnvGroupPath), "path to group.toml")
}

func FlagPlanPath(path *string) {
	flag.StringVar(path, "plan", os.Getenv(EnvPlanPath), "path to plan.toml")
}

func FlagRunImage(image *string) {
	flag.StringVar(image, "image", os.Getenv(EnvRunImage), "reference to run image")
}

func FlagUseDaemon(use *bool) {
	flag.BoolVar(use, "daemon", boolEnv(EnvUseDaemon), "export to docker daemon")
}

func FlagUseHelpers(use *bool) {
	flag.BoolVar(use, "helpers", boolEnv(EnvUseHelpers), "use credential helpers")
}

const (
	CodeFailed      = 1
	CodeInvalidArgs = iota + 2
	CodeInvalidEnv
	CodeNotFound
	CodeFailedDetect
	CodeFailedBuild
	CodeFailedLaunch
	CodeFailedUpdate
)

type ErrorFail struct {
	Err    error
	Code   int
	Action []string
}

func (e *ErrorFail) Error() string {
	message := "failed to " + strings.Join(e.Action, " ")
	if e.Err == nil {
		return message
	}
	return fmt.Sprintf("%s: %s", message, e.Err)
}

func FailCode(code int, action ...string) error {
	return FailErrCode(nil, code, action...)
}

func FailErr(err error, action ...string) error {
	code := CodeFailed
	if err, ok := err.(*ErrorFail); ok {
		code = err.Code
	}
	return FailErrCode(err, code, action...)
}

func FailErrCode(err error, code int, action ...string) error {
	return &ErrorFail{Err: err, Code: code, Action: action}
}

func Exit(err error) {
	if err == nil {
		os.Exit(0)
	}
	log.Printf("Error: %s\n", err)
	if err, ok := err.(*ErrorFail); ok {
		os.Exit(err.Code)
	}
	os.Exit(CodeFailed)
}

func boolEnv(k string) bool {
	v := os.Getenv(k)
	return v == "true" || v == "1"
}
