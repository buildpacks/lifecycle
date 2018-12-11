module github.com/buildpack/lifecycle

require (
	github.com/BurntSushi/toml v0.3.1
	github.com/containerd/continuity v0.0.0-20181203112020-004b46473808 // indirect
	github.com/docker/docker v0.7.3-0.20181027010111-b8e87cfdad8d
	github.com/docker/go-connections v0.4.0
	github.com/golang/mock v1.2.0
	github.com/google/go-cmp v0.2.0
	github.com/google/go-containerregistry v0.0.0-20181023232207-eb57122f1bf9
	github.com/opencontainers/runc v0.1.1 // indirect
	github.com/pkg/errors v0.8.0
	github.com/sclevine/spec v1.0.0
	github.com/sirupsen/logrus v1.2.0 // indirect
	github.com/stevvooe/resumable v0.0.0-20180830230917-22b14a53ba50 // indirect
	golang.org/x/net v0.0.0-20181201002055-351d144fa1fc // indirect
	golang.org/x/sys v0.0.0-20181128092732-4ed8d59d0b35 // indirect
	gotest.tools v2.2.0+incompatible // indirect
)

replace github.com/google/go-containerregistry v0.0.0-20181023232207-eb57122f1bf9 => github.com/dgodd/go-containerregistry v0.0.0-20180912122137-611aad063148a69435dccd3cf8475262c11814f6
