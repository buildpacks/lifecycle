module github.com/buildpacks/lifecycle/tools

go 1.15

require (
	github.com/BurntSushi/toml v0.3.1
	github.com/buildpacks/imgutil v0.0.0-20201211223552-8581300fe2b2
	github.com/buildpacks/lifecycle v0.9.2
	github.com/docker/docker v1.4.2-0.20190924003213-a8608b5b67c7
	github.com/golang/mock v1.4.4
	github.com/golangci/golangci-lint v1.30.0
	github.com/google/go-containerregistry v0.3.0
	github.com/pkg/errors v0.9.1
	github.com/sclevine/yj v0.0.0-20190506050358-d9a48607cc5c
	golang.org/x/tools v0.0.0-20200916195026-c9a70fc28ce3
)

replace github.com/buildpacks/lifecycle v0.9.2 => ../
