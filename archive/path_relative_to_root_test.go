package archive_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/buildpacks/lifecycle/archive"
)

func TestPathRelativeToRoot(t *testing.T) {
	root := filepath.Join("/tmp", "abc", "layers")
	tests := []struct {
		name    string
		path    string
		wantRel string
		wantErr string
		isDir   bool
	}{
		{"in-root file", filepath.Join(root, "sbom", "launch", "f"), filepath.Join("sbom", "launch", "f"), "", false},
		{"root itself", root, ".", "", true},
		{"ancestor dir allowed", filepath.Join("/tmp", "abc"), "", "", true},
		{"ancestor top dir allowed", "/tmp", "", "", true},
		{"ancestor as non-dir rejected", filepath.Join("/tmp", "abc"), "", "escapes destination root", false},
		{"escaping sibling rejected", "/home/cnb/.profile", "", "escapes destination root", true},
		{"prefix-not-ancestor rejected", filepath.Join("/tmp", "abc", "lay"), "", "escapes destination root", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rel, err := archive.PathRelativeToRoot(tc.path, root, tc.isDir)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected nil, got %v", err)
				}
				if rel != tc.wantRel {
					t.Fatalf("expected rel %q, got %q", tc.wantRel, rel)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}
