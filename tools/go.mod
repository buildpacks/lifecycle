module github.com/buildpacks/lifecycle/tools

go 1.14

require (
	github.com/BurntSushi/toml v0.3.1
	github.com/buildpacks/imgutil v0.0.0-20200831154319-afd98bd2f655
	github.com/buildpacks/lifecycle v0.8.1
	github.com/docker/docker v1.14.0-0.20190319215453-e7b5f7dbe98c
	github.com/golang/mock v1.4.4
	github.com/golangci/golangci-lint v1.30.0
	github.com/google/go-containerregistry v0.1.2-0.20200804170047-b0d31a182cf0
	github.com/sclevine/yj v0.0.0-20190506050358-d9a48607cc5c
	golang.org/x/tools v0.0.0-20200724022722-7017fd6b1305
)

replace (
	github.com/buildpacks/lifecycle v0.8.1 => ../
	github.com/containerd/containerd v1.4.0-0.20191014053712-acdcf13d5eaf => github.com/containerd/containerd v0.0.0-20191014053712-acdcf13d5eaf
	github.com/docker/docker v1.14.0-0.20190319215453-e7b5f7dbe98c => github.com/docker/docker v1.4.2-0.20200221181110-62bd5a33f707
	github.com/tonistiigi/fsutil v0.0.0-20190819224149-3d2716dd0a4d => github.com/tonistiigi/fsutil v0.0.0-20191018213012-0f039a052ca1
)
