module github.com/buildpacks/lifecycle

replace (
	github.com/containerd/containerd v1.4.0-0.20191014053712-acdcf13d5eaf => github.com/containerd/containerd v0.0.0-20191014053712-acdcf13d5eaf
	github.com/docker/docker v1.14.0-0.20190319215453-e7b5f7dbe98c => github.com/docker/docker v1.4.2-0.20200221181110-62bd5a33f707
	github.com/tonistiigi/fsutil v0.0.0-20190819224149-3d2716dd0a4d => github.com/tonistiigi/fsutil v0.0.0-20191018213012-0f039a052ca1
)

require (
	github.com/BurntSushi/toml v0.3.1
	github.com/GoogleContainerTools/kaniko v1.0.1-0.20200828004614-f20f49505162
	github.com/Microsoft/go-winio v0.4.15-0.20190919025122-fc70bd9a86b5 // indirect
	github.com/apex/log v1.3.0
	github.com/buildpacks/imgutil v0.0.0-20200831154319-afd98bd2f655
	github.com/docker/cli v0.0.0-20200312141509-ef2f64abbd37 // indirect
	github.com/docker/docker v1.14.0-0.20190319215453-e7b5f7dbe98c
	github.com/golang/mock v1.4.4
	github.com/google/go-cmp v0.5.2
	github.com/google/go-containerregistry v0.1.2-0.20200804170047-b0d31a182cf0
	github.com/heroku/color v0.0.6
	github.com/pkg/errors v0.9.1
	github.com/sclevine/spec v1.4.0
	github.com/sirupsen/logrus v1.6.0
	golang.org/x/sync v0.0.0-20200317015054-43a5402ce75a
	golang.org/x/sys v0.0.0-20200523222454-059865788121
	gotest.tools/v3 v3.0.2 // indirect
)

go 1.14
