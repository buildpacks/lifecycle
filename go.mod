module github.com/buildpacks/lifecycle

require (
	github.com/BurntSushi/toml v1.1.0
	github.com/apex/log v1.9.0
	github.com/buildpacks/imgutil v0.0.0-20220504154612-41b113050e2b
	github.com/docker/docker v20.10.15+incompatible
	github.com/golang/mock v1.6.0
	github.com/google/go-cmp v0.5.8
	github.com/google/go-containerregistry v0.8.0
	github.com/heroku/color v0.0.6
	github.com/pkg/errors v0.9.1
	github.com/sclevine/spec v1.4.0
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/sys v0.0.0-20211216021012-1d35b9e2eb4e
)

require (
	github.com/Azure/go-ansiterm v0.0.0-20210617225240-d185dfc1b5a1 // indirect
	github.com/Microsoft/go-winio v0.5.1 // indirect
	github.com/containerd/stargz-snapshotter/estargz v0.10.1 // indirect
	github.com/docker/cli v20.10.12+incompatible // indirect
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/docker/docker-credential-helpers v0.6.4 // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-units v0.4.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/klauspost/compress v1.13.6 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/moby/term v0.0.0-20210619224110-3f7ff695adc6 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.2 // indirect
	github.com/sirupsen/logrus v1.8.1 // indirect
	github.com/vbatts/tar-split v0.11.2 // indirect
	golang.org/x/net v0.0.0-20211216030914-fe4d6282115f // indirect
)

go 1.17

replace github.com/containerd/containerd => github.com/containerd/containerd v1.5.10
