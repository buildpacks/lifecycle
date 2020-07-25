package lifecycle

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/BurntSushi/toml"
	kanikoconfig "github.com/GoogleContainerTools/kaniko/pkg/config"
	//"github.com/GoogleContainerTools/kaniko/pkg/snapshot"
	kanikoutil "github.com/GoogleContainerTools/kaniko/pkg/util"

	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/snapshot"
)

type RootBuilder struct {
	RootDir       string
	LayersDir     string
	PlatformDir   string
	BuildpacksDir string
	Env           BuildEnv
	Group         BuildpackGroup
	Plan          BuildPlan
	Out, Err      *log.Logger
	Privileged    bool
}

type RootBuildEnv interface {
	AddRootDir(baseDir string) error
	AddEnvDir(envDir string) error
	WithPlatform(platformDir string) ([]string, error)
	List() []string
}

func (b *RootBuilder) Build() (*BuildMetadata, error) {
	platformDir, err := filepath.Abs(b.PlatformDir)
	if err != nil {
		return nil, err
	}
	layersDir, err := filepath.Abs(b.LayersDir)
	if err != nil {
		return nil, err
	}
	rootDir, err := filepath.Abs(b.RootDir)
	if err != nil {
		return nil, err
	}
	planDir, err := ioutil.TempDir("", "plan.")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(planDir)


	kanikoconfig.RootDir = rootDir
	kanikoDir, err := ioutil.TempDir("", "kaniko")
	if err != nil {
		return nil, err
	}
	kanikoconfig.KanikoDir = kanikoDir

	// TODO set kanikoconfig.IgnoreListPath
	kanikoutil.AddVolumePathToIgnoreList(filepath.Join(rootDir, "cnb"))
	kanikoutil.AddVolumePathToIgnoreList(filepath.Join(rootDir, "layers"))
	kanikoutil.AddVolumePathToIgnoreList(filepath.Join(rootDir, "tmp"))
	layeredMap := snapshot.NewLayeredMap(kanikoutil.Hasher(), kanikoutil.CacheHasher())
	snapshotter := snapshot.NewSnapshotter(layeredMap, b.RootDir)
	if err := snapshotter.Init(); err != nil {
		return nil, err
	}

	procMap := processMap{}
	plan := b.Plan
	var bom []BOMEntry
	var slices []Slice

	for _, bp := range b.Group.Group {
		bpInfo, err := bp.lookup(b.BuildpacksDir)
		if err != nil {
			return nil, err
		}
		bpDirName := launch.EscapeID(bp.ID)
		bpPlanDir := filepath.Join(planDir, bpDirName)

		if err := os.MkdirAll(bpPlanDir, 0777); err != nil {
			return nil, err
		}
		bpPlanPath := filepath.Join(bpPlanDir, "plan.toml")
		if err := WriteTOML(bpPlanPath, plan.find(bp)); err != nil {
			return nil, err
		}

		cmd := exec.Command(
			filepath.Join(bpInfo.Path, "bin", "build"),
			rootDir,
			platformDir,
			bpPlanPath,
		)
		cmd.Dir = rootDir
		cmd.Stdout = b.Out.Writer()
		cmd.Stderr = b.Err.Writer()

		if bpInfo.Buildpack.ClearEnv {
			cmd.Env = b.Env.List()
		} else {
			cmd.Env, err = b.Env.WithPlatform(platformDir)
			if err != nil {
				return nil, err
			}
		}
		cmd.Env = append(cmd.Env, EnvBuildpackDir+"="+bpInfo.Path)

		if err := cmd.Run(); err != nil {
			return nil, err
		}
		var bpPlanOut buildpackPlan
		if _, err := toml.DecodeFile(bpPlanPath, &bpPlanOut); err != nil {
			return nil, err
		}
		var bpBOM []BOMEntry
		plan, bpBOM = plan.filter(bp, bpPlanOut)
		bom = append(bom, bpBOM...)

		//var launch LaunchTOML
		//tomlPath := filepath.Join(rootDir, "launch.toml")
		//if _, err := toml.DecodeFile(tomlPath, &launch); os.IsNotExist(err) {
		//	continue
		//} else if err != nil {
		//	return nil, err
		//}
		//procMap.add(launch.Processes)
		//slices = append(slices, launch.Slices...)

		snapshotTmpFile, err := snapshotter.TakeSnapshotFS()
		if err != nil {
			return nil, err
		}

		snapshotLayerFile := filepath.Join(layersDir, fmt.Sprintf("%s.tgz", bpDirName))
		err = os.Rename(snapshotTmpFile, snapshotLayerFile)
		if err != nil {
			return nil, err
		}
	}

	return &BuildMetadata{
		Processes:  procMap.list(),
		Buildpacks: b.Group.Group,
		BOM:        bom,
		Slices:     slices,
	}, nil
}