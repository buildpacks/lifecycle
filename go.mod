module github.com/buildpack/lifecycle

require (
	github.com/BurntSushi/toml v0.3.1
	github.com/apex/log v1.1.2-0.20190827100214-baa5455d1012
	github.com/buildpack/imgutil v0.0.0-20191010153712-78959154ded1
	github.com/docker/docker v1.4.2-0.20190924003213-a8608b5b67c7
	github.com/docker/go-connections v0.4.0
	github.com/golang/mock v1.3.1
	github.com/google/go-cmp v0.3.0
	github.com/google/go-containerregistry v0.0.0-20191018211754-b77a90c667af
	github.com/heroku/color v0.0.6
	github.com/mattn/go-colorable v0.1.4 // indirect
	github.com/mattn/go-isatty v0.0.10 // indirect
	github.com/pkg/errors v0.8.1
	github.com/sclevine/spec v1.2.0
	golang.org/x/net v0.0.0-20190724013045-ca1201d0de80 // indirect
	google.golang.org/genproto v0.0.0-20190508193815-b515fa19cec8 // indirect
)

replace github.com/buildpack/imgutil => ../imgutil

go 1.13
