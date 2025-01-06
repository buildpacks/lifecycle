package archive_test

import (
	"archive/tar"
	"io"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/archive"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestReader(t *testing.T) {
	spec.Run(t, "tar", testNormalizingTarReader, spec.Report(report.Terminal{}))
}

func testNormalizingTarReader(t *testing.T, when spec.G, it spec.S) {
	when("NormalizingTarReader", func() {
		var (
			ftr *fakeTarReader
			ntr *archive.NormalizingTarReader
		)

		it.Before(func() {
			ftr = &fakeTarReader{}
			ntr = archive.NewNormalizingTarReader(ftr)
			ftr.pushHeader(&tar.Header{Name: "/some/path"})
		})

		it("converts path separators", func() {
			hdr, err := ntr.Next()
			h.AssertNil(t, err)
			h.AssertEq(t, hdr.Name, `/some/path`)
		})

		when("#Strip", func() {
			it("removes leading dirs", func() {
				ntr.Strip("/some")
				hdr, err := ntr.Next()
				h.AssertNil(t, err)
				h.AssertEq(t, hdr.Name, `/path`)
			})
		})

		when("#PrependDir", func() {
			it("prepends the dir", func() {
				ntr.PrependDir("/super-dir")
				hdr, err := ntr.Next()
				h.AssertNil(t, err)
				h.AssertEq(t, hdr.Name, `/super-dir/some/path`)
			})
		})

		when("#Exclude", func() {
			it("skips excluded entries", func() {
				ftr.pushHeader(&tar.Header{Name: "excluded-dir"})
				ftr.pushHeader(&tar.Header{Name: "excluded-dir/file"})
				ntr.ExcludePaths([]string{"excluded-dir"})
				hdr, err := ntr.Next()
				h.AssertNil(t, err)
				h.AssertEq(t, hdr.Name, `/some/path`)
			})
		})
	})
}

type fakeTarReader struct {
	hdrs []*tar.Header
}

func (r *fakeTarReader) Next() (*tar.Header, error) {
	if len(r.hdrs) == 0 {
		return nil, io.EOF
	}
	hdr := r.hdrs[0]
	r.hdrs = r.hdrs[1:]
	return hdr, nil
}

func (r *fakeTarReader) Read(b []byte) (int, error) {
	return 0, io.EOF
}

func (r *fakeTarReader) pushHeader(hdr *tar.Header) {
	r.hdrs = append([]*tar.Header{hdr}, r.hdrs...)
}
