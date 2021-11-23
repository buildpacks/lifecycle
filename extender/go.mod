module github.com/buildpacks/lifecycle/extender

go 1.16

require (
	github.com/BurntSushi/toml v0.3.1
	github.com/GoogleContainerTools/kaniko v1.7.0
	github.com/redhat-buildpacks/poc/kaniko v0.0.0-00010101000000-000000000000
)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v14.2.0+incompatible
	github.com/containerd/containerd v1.4.0-0.20191014053712-acdcf13d5eaf => github.com/containerd/containerd v0.0.0-20191014053712-acdcf13d5eaf
	github.com/docker/docker v1.14.0-0.20190319215453-e7b5f7dbe98c => github.com/docker/docker v0.0.0-20190319215453-e7b5f7dbe98c
	github.com/redhat-buildpacks/poc/kaniko => ./code
	github.com/tonistiigi/fsutil v0.0.0-20190819224149-3d2716dd0a4d => github.com/tonistiigi/fsutil v0.0.0-20191018213012-0f039a052ca1
)
