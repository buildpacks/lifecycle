package main

import (
	"fmt"
	cfg "github.com/redhat-buildpacks/poc/kaniko/buildpackconfig"
	"github.com/redhat-buildpacks/poc/kaniko/logging"
	util "github.com/redhat-buildpacks/poc/kaniko/util"
	logrus "github.com/sirupsen/logrus"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	LOGGING_LEVEL_ENV_NAME     = "LOGGING_LEVEL"
	LOGGING_FORMAT_ENV_NAME    = "LOGGING_FORMAT"
	LOGGING_TIMESTAMP_ENV_NAME = "LOGGING_TIMESTAMP"
	EXTRACT_LAYERS_ENV_NAME    = "EXTRACT_LAYERS"
	FILES_TO_SEARCH_ENV_NAME   = "FILES_TO_SEARCH"

	DefaultLevel        = "info"
	DefaultLogTimestamp = false
	DefaultLogFormat    = "text"
)

var (
	logLevel      string   // Log level (trace, debug, info, warn, error, fatal, panic)
	logFormat     string   // Log format (text, color, json)
	logTimestamp  bool     // Timestamp in log output
	extractLayers bool     // Extract layers from tgz files. Default is false
	filesToSearch []string // List of files to search to check if they exist under the updated FS
)

func init() {
	envVal := util.GetValFromEnVar(FILES_TO_SEARCH_ENV_NAME)
	if envVal != "" {
		filesToSearch = strings.Split(envVal, ",")
	}

	logLevel = util.GetValFromEnVar(LOGGING_LEVEL_ENV_NAME)
	if logLevel == "" {
		logLevel = DefaultLevel
	}

	logFormat = util.GetValFromEnVar(LOGGING_FORMAT_ENV_NAME)
	if logFormat == "" {
		logFormat = DefaultLogFormat
	}

	loggingTimeStampStr := util.GetValFromEnVar(LOGGING_TIMESTAMP_ENV_NAME)
	if loggingTimeStampStr == "" {
		logTimestamp = DefaultLogTimestamp
	} else {
		v, err := strconv.ParseBool(loggingTimeStampStr)
		if err != nil {
			logrus.Fatalf("logTimestamp bool assignment failed %s", err)
		} else {
			logTimestamp = v
		}
	}

	extractLayersStr := util.GetValFromEnVar(EXTRACT_LAYERS_ENV_NAME)
	if extractLayersStr == "" {
		logrus.Info("The layered tzg files will NOT be extracted to the home dir ...")
		extractLayers = false
	} else {
		v, err := strconv.ParseBool(extractLayersStr)
		if err != nil {
			logrus.Fatalf("extractLayers bool assignment failed %s", err)
		} else {
			extractLayers = v
			logrus.Info("The layered tzg files will be extracted to the home dir ...")
		}
	}

	if err := logging.Configure(logLevel, logFormat, logTimestamp); err != nil {
		panic(err)
	}
}

func main() {
	if _, ok := os.LookupEnv("DEBUG"); ok && (len(os.Args) <= 1 || os.Args[1] != "from-debugger") {
		args := []string {
			"--listen=:2345",
			"--headless=true",
			"--api-version=2",
			"--accept-multiclient",
			"exec",
			"/kaniko-app", "from-debugger",
		}
		err := syscall.Exec("/usr/local/bin/dlv", append([]string{"/usr/local/bin/dlv"}, args...), os.Environ())
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}
	logrus.Info("Starting the Kaniko application to process a Dockerfile ...")

	// Create a buildPackConfig and set the default values
	logrus.Info("Initialize the BuildPackConfig and set the defaults values ...")
	b := cfg.NewBuildPackConfig()
	b.InitDefaults()
	b.ExtractLayers = extractLayers

	logrus.Infof("Kaniko      dir: %s", b.KanikoDir)
	logrus.Infof("Workspace   dir: %s", b.WorkspaceDir)
	logrus.Infof("Cache       dir: %s", b.CacheDir)
	logrus.Infof("Dockerfile name: %s", b.DockerFileName)
	logrus.Infof("Extract layer files ? %v", extractLayers)

	// Launch a timer to measure the time needed to parse/copy/extract
	start := time.Now()

	// Build the Dockerfile
	logrus.Infof("Building the %s", b.DockerFileName)
	err := b.BuildDockerFile()
	if err != nil {
		panic(err)
	}
	err = reapChildProcesses()
	if err != nil {
		panic(err)
	}

	// Log the content of the Kaniko dir
	logrus.Infof("Reading dir content of: %s", b.KanikoDir)
	util.ReadFilesFromPath(b.KanikoDir)

	// Copy the tgz layer file to the Cache dir
	srcPath := path.Join("/", b.LayerTarFileName)
	dstPath := path.Join(b.CacheDir, b.LayerTarFileName)
	logrus.Infof("Copy the %s file to the %s dir ...", srcPath, dstPath)
	err = util.File(srcPath, dstPath)
	if (err != nil) {
		panic(err)
	}

	logrus.Infof("Extract the content of the tarball file %s under the cache %s",b.Opts.TarPath,b.Opts.CacheDir)
	b.ExtractImageTarFile(dstPath)
	logrus.Info("Extract the layer file(s)")
	//baseImageHash := b.FindBaseImageDigest()
	//logrus.Infof("Hash of the base image is: %s",baseImageHash.String())
	descriptor, err := b.LoadDescriptorAndConfig()
	if (err != nil) {
		panic(err)
	}

	logrus.Infof("%+v\n", descriptor)
	layers := descriptor[0].Layers
	/*	for i := 1; i < len(layers); i++ {
		logrus.Infof("Layer: %s", layers[i])
	}*/
	b.ExtractTarGZFilesWithoutBaseImage(layers[0])

	// Check if files exist
	if (len(filesToSearch) > 0) {
		util.FindFiles(filesToSearch)
	}

	// Time elapsed is ...
	logrus.Infof("Time elapsed: %s",time.Since(start))
}

func reapChildProcesses() error {
	procDir, err := os.Open("/proc")
	if err != nil {
		return err
	}

	procDirs, err := procDir.Readdirnames(-1)
	if err != nil {
		return err
	}

	tid := os.Getpid()

	var wg sync.WaitGroup
	for _, dirName := range procDirs {
		pid, err := strconv.Atoi(dirName)
		if err == nil && pid != 1 && pid != tid {
			p, err := os.FindProcess(pid)
			if err != nil {
				continue
			}
			err = p.Signal(syscall.SIGTERM)
			if err != nil {
				continue
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				p.Wait()
			}()
		}
	}
	wg.Wait()
	return nil
}