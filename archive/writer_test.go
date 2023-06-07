package archive_test

import (
	"archive/tar"
	"runtime"
	"testing"
	"time"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/archive"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestWriter(t *testing.T) {
	spec.Run(t, "tar", testNormalizingTarWriter, spec.Report(report.Terminal{}))
}

func testNormalizingTarWriter(t *testing.T, when spec.G, it spec.S) {
	when("NormalizingTarWriter", func() {
		var (
			ftw *fakeTarWriter
			ntw *archive.NormalizingTarWriter
		)

		it.Before(func() {
			ftw = &fakeTarWriter{}
			ntw = archive.NewNormalizingTarWriter(ftw)
		})

		it("sets empty Uname and Gname", func() {
			h.AssertNil(t, ntw.WriteHeader(&tar.Header{
				Uname: "some-uname",
				Gname: "some-gname",
			}))
			h.AssertEq(t, ftw.getLastHeader().Uname, "")
			h.AssertEq(t, ftw.getLastHeader().Gname, "")
		})

		when("windows", func() {
			it.Before(func() {
				if runtime.GOOS != "windows" {
					t.Skip("windows specific test")
				}
			})

			it("converts path separators", func() {
				h.AssertNil(t, ntw.WriteHeader(&tar.Header{
					Name: `c:\some\file\path`,
				}))
				h.AssertEq(t, ftw.getLastHeader().Name, "/some/file/path")
			})
		})

		when("#WithUID", func() {
			it("sets the uid", func() {
				ntw.WithUID(999)
				h.AssertNil(t, ntw.WriteHeader(&tar.Header{
					Uid: 888,
				}))
				h.AssertEq(t, ftw.getLastHeader().Uid, 999)
			})
		})

		when("#WithGID", func() {
			it("sets the gid", func() {
				ntw.WithGID(999)
				h.AssertNil(t, ntw.WriteHeader(&tar.Header{
					Gid: 888,
				}))
				h.AssertEq(t, ftw.getLastHeader().Gid, 999)
			})
		})

		when("#WithModTime", func() {
			it("sets the mod time", func() {
				ntw.WithModTime(archive.NormalizedModTime)
				h.AssertNil(t, ntw.WriteHeader(&tar.Header{
					ModTime: time.Now(),
				}))
				if !ftw.getLastHeader().ModTime.Equal(time.Date(1980, time.January, 1, 0, 0, 1, 0, time.UTC)) {
					t.Fatalf("failed to normalize the mod time")
				}
			})
		})
	})
}

type fakeTarWriter struct {
	hdr *tar.Header
}

func (w *fakeTarWriter) WriteHeader(hdr *tar.Header) error {
	w.hdr = hdr
	return nil
}
func (w *fakeTarWriter) getLastHeader() *tar.Header {
	return w.hdr
}

func (w *fakeTarWriter) Write(b []byte) (int, error) {
	return len(b), nil
}

func (w *fakeTarWriter) Close() error {
	return nil
}
