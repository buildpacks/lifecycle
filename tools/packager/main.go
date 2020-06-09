package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"text/template"
)

func main() {
	var CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	outputPackagePathPtr := CommandLine.String("outputPackagePath", "", "")
	lifecycleExePathPtr := CommandLine.String("lifecycleExePath", "", "")
	launcherExePathPtr := CommandLine.String("launcherExePath", "", "")
	platformAPIPtr := CommandLine.String("platformAPI", "", "")
	buildpackAPIPtr := CommandLine.String("buildpackAPI", "", "")
	lifecycleVersionPtr := CommandLine.String("lifecycleVersion", "", "")
	osPtr := CommandLine.String("os", "", "")

	if err := CommandLine.Parse(os.Args[1:]); err != nil {
		fmt.Fprintf(CommandLine.Output(), "ERROR: %s\n", err.Error())
		os.Exit(1)
	}

	flagsMissing := false
	CommandLine.VisitAll(func(f *flag.Flag) {
		if f.Value.String() == "" {
			fmt.Fprintf(CommandLine.Output(), "%s can't be blank\n", f.Name)
			flagsMissing = true
		}
	})

	if flagsMissing == true {
		fmt.Fprintf(CommandLine.Output(), "Usage of %s:\n", os.Args[0])
		CommandLine.PrintDefaults()
		os.Exit(1)
	}

	packager := NewPackager(
		*lifecycleExePathPtr,
		*launcherExePathPtr,
		*platformAPIPtr,
		*buildpackAPIPtr,
		*lifecycleVersionPtr,
		*osPtr,
	)

	if err := packager.WriteToTarGzPath(*outputPackagePathPtr); err != nil {
		fmt.Fprintf(CommandLine.Output(), "ERROR: %s\n", err.Error())
		os.Exit(1)
	}
}

type packager struct {
	lifecycleExePath, launcherExePath, platformAPI, buildpackAPI, lifecycleVersion, exeSuffix string
}

func NewPackager(lifecycleExePath, launcherExePath, platformAPI, buildpackAPI, lifecycleVersion, os string) *packager {
	exeSuffix := ""
	if os == "windows" {
		exeSuffix = ".exe"
	}

	return &packager{lifecycleExePath, launcherExePath, platformAPI, buildpackAPI, lifecycleVersion, exeSuffix}
}

func (p *packager) WriteToTarGzPath(outputPath string) error {
	outputTarGzFile, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer outputTarGzFile.Close()

	gzipWriter := gzip.NewWriter(outputTarGzFile)
	defer gzipWriter.Close()
	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	lifecycleData, err := lifecycleTemplateData(p.platformAPI, p.buildpackAPI, p.lifecycleVersion)
	if err != nil {
		return err
	}
	if err := addDataEntry(tarWriter, lifecycleData, "lifecycle.toml", 0444); err != nil {
		return err
	}
	if err := tarWriter.WriteHeader(&tar.Header{Name: "lifecycle/", Typeflag: tar.TypeDir}); err != nil {
		return err
	}
	if err := addFileEntry(tarWriter, p.lifecycleExePath, "lifecycle/lifecycle"+p.exeSuffix, 0555); err != nil {
		return err
	}
	if err := addFileEntry(tarWriter, p.launcherExePath, "lifecycle/launcher"+p.exeSuffix, 0555); err != nil {
		return err
	}
	if err := addSymlinkEntry(tarWriter, "lifecycle"+p.exeSuffix, "lifecycle/creator"+p.exeSuffix); err != nil {
		return err
	}
	if err := addSymlinkEntry(tarWriter, "lifecycle"+p.exeSuffix, "lifecycle/rebaser"+p.exeSuffix); err != nil {
		return err
	}
	if err := addSymlinkEntry(tarWriter, "lifecycle"+p.exeSuffix, "lifecycle/detector"+p.exeSuffix); err != nil {
		return err
	}
	if err := addSymlinkEntry(tarWriter, "lifecycle"+p.exeSuffix, "lifecycle/exporter"+p.exeSuffix); err != nil {
		return err
	}
	if err := addSymlinkEntry(tarWriter, "lifecycle"+p.exeSuffix, "lifecycle/builder"+p.exeSuffix); err != nil {
		return err
	}
	if err := addSymlinkEntry(tarWriter, "lifecycle"+p.exeSuffix, "lifecycle/analyzer"+p.exeSuffix); err != nil {
		return err
	}
	if err := addSymlinkEntry(tarWriter, "lifecycle"+p.exeSuffix, "lifecycle/restorer"+p.exeSuffix); err != nil {
		return err
	}

	return nil
}

func lifecycleTemplateData(platformAPI, buildpackAPI, lifecycleVersion string) ([]byte, error) {
	data := &bytes.Buffer{}
	err := template.Must(template.New("tmpl").Parse(
		strings.TrimSpace(`
[api]
platform = "{{.PlatformAPI}}"
buildpack = "{{.BuildpackAPI}}"

[lifecycle]
version = "{{.LifecycleVersion}}"
`),
	)).Execute(data, struct {
		PlatformAPI, BuildpackAPI, LifecycleVersion string
	}{
		platformAPI, buildpackAPI, lifecycleVersion,
	})

	if err != nil {
		return nil, err
	}

	return data.Bytes(), nil
}

func addFileEntry(tarWriter *tar.Writer, filePath, tarEntryPath string, mode int64) error {
	fileData, err := ioutil.ReadFile(filePath)
	if err != nil {
		return err
	}

	return addDataEntry(tarWriter, fileData, tarEntryPath, mode)
}

func addDataEntry(tarWriter *tar.Writer, fileData []byte, tarEntryPath string, mode int64) error {
	fileHeader := &tar.Header{Name: tarEntryPath, Size: int64(len(fileData)), Mode: mode}
	if err := tarWriter.WriteHeader(fileHeader); err != nil {
		return err
	}

	if _, err := tarWriter.Write(fileData); err != nil {
		return err
	}
	return nil
}

func addSymlinkEntry(tarWriter *tar.Writer, linkname, name string) error {
	header := &tar.Header{Name: name, Linkname: linkname, Typeflag: tar.TypeSymlink}
	if err := tarWriter.WriteHeader(header); err != nil {
		return err
	}
	return nil
}
