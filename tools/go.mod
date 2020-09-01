module github.com/buildpacks/lifecycle/tools

go 1.14

require (
	github.com/BurntSushi/toml v0.3.1
	github.com/buildpacks/imgutil v0.0.0-20200831154319-afd98bd2f655
	github.com/buildpacks/lifecycle v0.8.1
	github.com/docker/docker v1.4.2-0.20200221181110-62bd5a33f707
	github.com/golang/mock v1.4.4
	github.com/golangci/golangci-lint v1.30.0
	github.com/google/go-containerregistry v0.0.0-20200313165449-955bf358a3d8
	github.com/sclevine/yj v0.0.0-20190506050358-d9a48607cc5c
	golang.org/x/tools v0.0.0-20200724022722-7017fd6b1305
)

replace github.com/buildpacks/lifecycle v0.8.1 => ../
