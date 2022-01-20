module github.com/redhat-buildpacks/poc/kaniko

require (
	github.com/GoogleContainerTools/kaniko v1.7.0
	github.com/google/go-containerregistry v0.4.1-0.20210128200529-19c2b639fab1
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.8.1
	github.com/stretchr/testify v1.6.1
	gotest.tools v2.2.0+incompatible // indirect
)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v14.2.0+incompatible
	github.com/containerd/containerd v1.4.0-0.20191014053712-acdcf13d5eaf => github.com/containerd/containerd v0.0.0-20191014053712-acdcf13d5eaf
	github.com/docker/docker v1.14.0-0.20190319215453-e7b5f7dbe98c => github.com/docker/docker v0.0.0-20190319215453-e7b5f7dbe98c
	github.com/tonistiigi/fsutil v0.0.0-20190819224149-3d2716dd0a4d => github.com/tonistiigi/fsutil v0.0.0-20191018213012-0f039a052ca1
)

go 1.16
