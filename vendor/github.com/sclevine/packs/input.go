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

	EnvDropletPath    = "PACK_DROPLET_PATH"
	EnvSlugPath       = "PACK_SLUG_PATH"
	EnvMetadataPath   = "PACK_METADATA_PATH"
	EnvBPListPath     = "PACK_BP_LIST_PATH"
	EnvBPOrderPath    = "PACK_BP_ORDER_PATH"
	EnvBPGroupPath    = "PACK_BP_GROUP_PATH"
	EnvDetectInfoPath = "PACK_DETECT_INFO_PATH"

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

func InputBPListPath(path *string) {
	flag.StringVar(path, "list", os.Getenv(EnvBPListPath), "file containing list of buildpacks")
}

func InputBPOrderPath(path *string) {
	flag.StringVar(path, "order", os.Getenv(EnvBPOrderPath), "file containing detection order")
}

func InputBPGroupPath(path *string) {
	flag.StringVar(path, "group", os.Getenv(EnvBPGroupPath), "file containing a buildpack group")
}

func InputDetectInfoPath(path *string) {
	flag.StringVar(path, "info", os.Getenv(EnvDetectInfoPath), "file containing detection info")
}

func InputStackName(name *string) {
	flag.StringVar(name, "stack", os.Getenv(EnvStackName), "image repository containing stack")
}

func InputUseDaemon(use *bool) {
	flag.BoolVar(use, "daemon", BoolEnv(EnvUseDaemon), "export to docker daemon")
}
