package layers

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/archive"
	"github.com/buildpacks/lifecycle/launch"
)

const (
	processDir   = "/cnb/process"
	launcherPath = "/cnb/lifecycle/launcher"
)

// LauncherLayer creates a Layer containing the launcher at the given path
func (f *Factory) LauncherLayer(path string) (layer Layer, err error) {
	parents := []*tar.Header{
		rootOwnedDir("/cnb"),
		rootOwnedDir("/cnb/lifecycle"),
	}
	fi, err := os.Stat(path)
	if err != nil {
		return Layer{}, fmt.Errorf("failed to stat launcher at path '%s'", path)
	}
	hdr, err := tar.FileInfoHeader(fi, "")
	if err != nil {
		return Layer{}, fmt.Errorf("failed create TAR header for launcher at path '%s'", path)
	}
	hdr.Name = launcherPath
	hdr.Uid = 0
	hdr.Gid = 0
	hdr.Mode = 0755

	return f.writeLayer("launcher", func(tw *archive.NormalizingTarWriter) error {
		for _, dir := range parents {
			if err := tw.WriteHeader(dir); err != nil {
				return err
			}
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return errors.Wrap(err, "failed to write header for launcher")
		}

		lf, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open launcher at path '%s'", path)
		}
		defer lf.Close()
		n, err := io.Copy(tw, lf)
		fmt.Println("wrote bytes", n)
		if err != nil {
			return errors.Wrap(err, "failed to write launcher to layer")
		}
		return nil
	})
}

// SymlinksLayers creates a Layer containing symlinks pointing to target where:
//    * any parents of the symlink files will also be added to the layer
//    * symlinks and their parent directories shall be root owned and world readable
func (f *Factory) ProcessTypesLayer(config launch.Metadata) (layer Layer, err error) {
	hdrs := []*tar.Header{
		rootOwnedDir("/cnb"),
		rootOwnedDir(processDir),
	}
	for _, proc := range config.Processes {
		if len(proc.Type) == 0 {
			return Layer{}, errors.New("type is required for all processes")
		}
		if err := validateProcessType(proc.Type); err != nil {
			return Layer{}, errors.Wrapf(err, "invalid process type '%s'", proc.Type)
		}
		hdrs = append(hdrs, typeSymlink(proc.Type))
	}

	return f.writeLayer("process-types", func(tw *archive.NormalizingTarWriter) error {
		for _, hdr := range hdrs {
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
		}
		return nil
	})
}

func validateProcessType(pType string) error {
	forbiddenCharacters := `/><:|&\`
	if strings.ContainsAny(pType, forbiddenCharacters) {
		return fmt.Errorf(`type may not contain characters '%s'`, forbiddenCharacters)
	}
	return nil
}

func rootOwnedDir(path string) *tar.Header {
	return &tar.Header{
		Typeflag: tar.TypeDir,
		Name:     path,
		Mode:     int64(0755 | os.ModeDir),
	}
}

func typeSymlink(processType string) *tar.Header {
	return &tar.Header{
		Typeflag: tar.TypeSymlink,
		Name:     filepath.Join(processDir, processType),
		Linkname: launcherPath,
		Mode:     int64(0755 | os.ModeSymlink),
	}
}
