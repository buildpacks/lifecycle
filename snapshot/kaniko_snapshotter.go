package snapshot

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"

	kanikoconfig "github.com/GoogleContainerTools/kaniko/pkg/config"
	kanikoutil "github.com/GoogleContainerTools/kaniko/pkg/util"
)

type KanikoSnapshotter struct {
	RootDir     string
	snapshotter *Snapshotter
}

func NewKanikoSnapshotter(rootDir string) (*KanikoSnapshotter, error) {
	ls := KanikoSnapshotter{
		RootDir: rootDir,
	}
	if err := ls.Init(); err != nil {
		return nil, err
	}
	return &ls, nil
}

func (ls *KanikoSnapshotter) Init() error {
	logrus.SetLevel(logrus.FatalLevel)
	kanikoconfig.RootDir = ls.RootDir
	kanikoDir, err := ioutil.TempDir("", "kaniko")
	if err != nil {
		return err
	}
	kanikoconfig.KanikoDir = kanikoDir

	kanikoutil.AddVolumePathToIgnoreList(filepath.Join(ls.RootDir, "cnb"))
	kanikoutil.AddVolumePathToIgnoreList(filepath.Join(ls.RootDir, "layers"))
	kanikoutil.AddVolumePathToIgnoreList(filepath.Join(ls.RootDir, "tmp"))
	kanikoutil.AddVolumePathToIgnoreList(filepath.Join(ls.RootDir, "proc"))

	layeredMap := NewLayeredMap(kanikoutil.Hasher(), kanikoutil.CacheHasher())
	ls.snapshotter = NewSnapshotter(layeredMap, ls.RootDir)
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
