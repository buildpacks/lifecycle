// +build linux darwin

package snapshot

import (
	"io"
	"io/ioutil"
	"os"

	"github.com/sirupsen/logrus"

	"github.com/buildpacks/lifecycle"

	kconfig "github.com/GoogleContainerTools/kaniko/pkg/config"
	ksnap "github.com/GoogleContainerTools/kaniko/pkg/snapshot"
	kutil "github.com/GoogleContainerTools/kaniko/pkg/util"
)

type KanikoSnapshotter struct {
	RootDir                    string
	DetectFilesystemIgnoreList bool
	IgnoredPaths               []string
	snapshotter                *ksnap.Snapshotter
}

func NewKanikoSnapshotter(rootDir string) (lifecycle.LayerSnapshotter, error) {
	ls := KanikoSnapshotter{
		RootDir:                    rootDir,
		DetectFilesystemIgnoreList: true,
		IgnoredPaths: []string{
			"/tmp",
			"/layers",
			"/cnb",
		},
	}
	return &ls, nil
}

func (ls *KanikoSnapshotter) Init() error {
	logrus.SetLevel(logrus.FatalLevel)
	kconfig.RootDir = ls.RootDir
	kanikoDir, err := ioutil.TempDir("", "kaniko")
	if err != nil {
		return err
	}
	kconfig.KanikoDir = kanikoDir

	if ls.DetectFilesystemIgnoreList {
		if err := kutil.DetectFilesystemIgnoreList(kconfig.IgnoreListPath); err != nil {
			return err
		}
	}

	for _, e := range ignoreList(ls.IgnoredPaths) {
		kutil.AddToIgnoreList(e)
	}

	layeredMap := ksnap.NewLayeredMap(kutil.Hasher(), kutil.CacheHasher())
	ls.snapshotter = ksnap.NewSnapshotter(layeredMap, ls.RootDir)
	if err := ls.snapshotter.Init(); err != nil {
		return err
	}
	return nil
}

func (ls *KanikoSnapshotter) TakeSnapshot(snapshotLayerFile string) error {
	snapshotTmpFile, err := ls.snapshotter.TakeSnapshotFS()
	if err != nil {
		return err
	}

	in, err := os.Open(snapshotTmpFile)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(snapshotLayerFile)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	return out.Close()
}

func ignoreList(pathsToIgnore []string) []kutil.IgnoreListEntry {
	result := []kutil.IgnoreListEntry{}

	for _, path := range pathsToIgnore {
		result = append(result, kutil.IgnoreListEntry{
			Path:            path,
			PrefixMatchOnly: true,
		})
	}

	return result
}
