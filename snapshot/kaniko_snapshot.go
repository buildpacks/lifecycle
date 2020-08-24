package snapshot

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"

	kconfig "github.com/GoogleContainerTools/kaniko/pkg/config"
	ksnap "github.com/GoogleContainerTools/kaniko/pkg/snapshot"
	kutil "github.com/GoogleContainerTools/kaniko/pkg/util"
)

type KanikoSnapshotter struct {
	RootDir     string
	snapshotter *ksnap.Snapshotter
}

func NewKanikoSnapshotter(rootDir string) (*KanikoSnapshotter, error) {
	ls := KanikoSnapshotter{
		RootDir: rootDir,
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

	kutil.AddVolumePathToIgnoreList(filepath.Join(ls.RootDir, "cnb"))
	kutil.AddVolumePathToIgnoreList(filepath.Join(ls.RootDir, "layers"))
	kutil.AddVolumePathToIgnoreList(filepath.Join(ls.RootDir, "tmp"))
	kutil.AddVolumePathToIgnoreList(filepath.Join(ls.RootDir, "proc"))
	kutil.AddVolumePathToIgnoreList(filepath.Join(ls.RootDir, "sys"))

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
