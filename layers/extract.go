package layers

import (
	"archive/tar"
	"io"
	"path/filepath"

	"github.com/buildpacks/lifecycle/archive"
)

// Extract extracts entries from r to the dest directory.
// Contents of r should be an OCI layer. Entry paths are joined to dest, so an entry
// recorded as /layers/sbom/launch lands at <dest>/layers/sbom/launch.
// Any entry that would land outside dest is rejected.
// If dest is an empty string files will be extracted to `/` on unix filesystems.
func Extract(r io.Reader, dest string) error {
	root := cleanRoot(dest)
	tr := tarReader(r, root)
	return archive.Extract(tr, root)
}

// ExtractConfined extracts an OCI layer from r. Layer entries carry absolute paths, so files
// land at their recorded location. confineTo restricts extraction to descendants of
// confineTo: any entry outside it is rejected. An empty confineTo confines to "/"
// (no effective confinement).
func ExtractConfined(r io.Reader, confineTo string) error {
	root := cleanRoot(confineTo)
	tr := tarReader(r, `/`) // no-op for the absolute paths OCI layers carry
	return archive.Extract(tr, root)
}

func cleanRoot(dir string) string {
	if dir == "" {
		// Intentional documented fallback: no confinement.
		dir = `/`
	}
	return filepath.Clean(dir)
}

func tarReader(r io.Reader, dest string) archive.TarReader {
	tr := archive.NewNormalizingTarReader(tar.NewReader(r))
	tr.PrependDir(dest)
	return tr
}
