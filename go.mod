module github.com/buildpacks/lifecycle

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.3.3+incompatible
	github.com/containerd/containerd v1.4.0-0.20191014053712-acdcf13d5eaf => github.com/containerd/containerd v0.0.0-20191014053712-acdcf13d5eaf
	github.com/docker/docker v1.14.0-0.20190319215453-e7b5f7dbe98c => github.com/docker/docker v1.4.2-0.20200221181110-62bd5a33f707
	github.com/tonistiigi/fsutil v0.0.0-20190819224149-3d2716dd0a4d => github.com/tonistiigi/fsutil v0.0.0-20191018213012-0f039a052ca1
)

require (
	github.com/BurntSushi/toml v0.3.1
	github.com/GoogleContainerTools/kaniko v0.24.0
	github.com/Microsoft/go-winio v0.4.15-0.20190919025122-fc70bd9a86b5 // indirect
	github.com/apex/log v1.1.2-0.20190827100214-baa5455d1012
	github.com/buildpacks/imgutil v0.0.0-20200625161542-2281cd9b1414
	github.com/docker/cli v0.0.0-20200312141509-ef2f64abbd37
	github.com/docker/docker v1.14.0-0.20190319215453-e7b5f7dbe98c
	github.com/docker/go-connections v0.4.0
	github.com/gogo/protobuf v1.3.1 // indirect
	github.com/golang/mock v1.4.3
	github.com/golang/protobuf v1.3.5 // indirect
	github.com/google/go-cmp v0.5.0
	github.com/google/go-containerregistry v0.0.0-20200313165449-955bf358a3d8
	github.com/heroku/color v0.0.6
	github.com/kr/text v0.2.0 // indirect
	github.com/mattn/go-colorable v0.1.6 // indirect
	github.com/niemeyer/pretty v0.0.0-20200227124842-a10e7caefd8e // indirect
	github.com/pkg/errors v0.9.1
	github.com/sclevine/spec v1.4.0
	golang.org/x/crypto v0.0.0-20200311171314-f7b00557c8c4
	golang.org/x/net v0.0.0-20200301022130-244492dfa37a // indirect
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e
	golang.org/x/sys v0.0.0-20200302150141-5c8b2ff67527
	golang.org/x/time v0.0.0-20191024005414-555d28b269f0 // indirect
	google.golang.org/genproto v0.0.0-20200313141609-30c55424f95d // indirect
	google.golang.org/grpc v1.28.0 // indirect
	gopkg.in/check.v1 v1.0.0-20200227125254-8fa46927fb4f // indirect
	gotest.tools/v3 v3.0.2 // indirect
	vbom.ml/util v0.0.0-20180919145318-efcd4e0f9787 // indirect
)

go 1.14
