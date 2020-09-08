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
	RootDir     string
	snapshotter *ksnap.Snapshotter
	IgnoreList  IgnoreList
}

func NewKanikoSnapshotter(rootDir string) (lifecycle.LayerSnapshotter, error) {
	ls := KanikoSnapshotter{
		RootDir:    rootDir,
		IgnoreList: KanikoFilesystemIgnoreList{},
	}
	return &ls, nil
}

type IgnoreList interface {
	Load() error
	CustomEntries() []kutil.IgnoreListEntry
}

type KanikoFilesystemIgnoreList struct {
}

func (i KanikoFilesystemIgnoreList) Load() error {
	return kutil.DetectFilesystemIgnoreList(kconfig.IgnoreListPath)
}

func (i KanikoFilesystemIgnoreList) CustomEntries() []kutil.IgnoreListEntry {
	return []kutil.IgnoreListEntry{
		{
			Path:            "/tmp",
			PrefixMatchOnly: true,
		},
		{
			Path:            "/layers",
			PrefixMatchOnly: true,
		},
		{
			Path:            "/cnb",
			PrefixMatchOnly: true,
		},
	}
}

func (ls *KanikoSnapshotter) Init() error {
	logrus.SetLevel(logrus.FatalLevel)
	kconfig.RootDir = ls.RootDir
	kanikoDir, err := ioutil.TempDir("", "kaniko")
	if err != nil {
		return err
	}
	kconfig.KanikoDir = kanikoDir

	if err := ls.IgnoreList.Load(); err != nil {
		return err
	}

	for _, e := range ls.IgnoreList.CustomEntries() {
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

func (ls *KanikoSnapshotter) GetRootDir() string {
	return ls.RootDir
}
