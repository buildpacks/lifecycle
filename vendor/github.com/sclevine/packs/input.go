package packs

import (
	"flag"
	"os"
)

const (
	EnvAppDir = "PACK_APP_DIR"
	EnvAppZip = "PACK_APP_ZIP"

	EnvAppName = "PACK_APP_NAME"
	EnvAppURI  = "PACK_APP_URI"

	EnvAppDisk   = "PACK_APP_DISK"
	EnvAppMemory = "PACK_APP_MEM"
	EnvAppFds    = "PACK_APP_FDS"

	EnvDropletPath  = "PACK_DROPLET_PATH"
	EnvSlugPath     = "PACK_SLUG_PATH"
	EnvMetadataPath = "PACK_METADATA_PATH"
	EnvGroupPath    = "PACK_GROUP_PATH"
	EnvListPath     = "PACK_LIST_PATH"

	EnvStackName = "PACK_STACK_NAME"
	EnvUseDaemon = "PACK_USE_DAEMON"
)

func InputDropletPath(path *string) {
	flag.StringVar(path, "droplet", os.Getenv(EnvDropletPath), "file containing compressed droplet")
}

func InputSlugPath(path *string) {
	flag.StringVar(path, "slug", os.Getenv(EnvSlugPath), "file containing compressed slug")
}

func InputMetadataPath(path *string) {
	flag.StringVar(path, "metadata", os.Getenv(EnvMetadataPath), "file containing build metadata")
}

func InputGroupPath(path *string) {
	flag.StringVar(path, "group", os.Getenv(EnvGroupPath), "file containing buildpack group")
}

func InputListPath(path *string) {
	flag.StringVar(path, "list", os.Getenv(EnvListPath), "file containing list of buildpack groups")
}

func InputStackName(name *string) {
	flag.StringVar(name, "stack", os.Getenv(EnvStackName), "image repository containing stack")
}

func InputUseDaemon(use *bool) {
	flag.BoolVar(use, "daemon", BoolEnv(EnvUseDaemon), "export to docker daemon")
}
