module github.com/buildpacks/lifecycle/tools

go 1.15

require (
	github.com/BurntSushi/toml v0.3.1
	github.com/Djarvur/go-err113 v0.1.0 // indirect
	github.com/buildpacks/imgutil v0.0.0-20201008151938-cea9fc548372
	github.com/buildpacks/lifecycle v0.8.1
	github.com/docker/docker v1.4.2-0.20200221181110-62bd5a33f707
	github.com/golang/mock v1.4.4
	github.com/golangci/golangci-lint v1.30.0
	github.com/golangci/misspell v0.3.5 // indirect
	github.com/golangci/revgrep v0.0.0-20180812185044-276a5c0a1039 // indirect
	github.com/google/go-containerregistry v0.1.3
	github.com/jirfag/go-printf-func-name v0.0.0-20200119135958-7558a9eaa5af // indirect
	github.com/mitchellh/mapstructure v1.3.1 // indirect
	github.com/pelletier/go-toml v1.8.0 // indirect
	github.com/pkg/errors v0.9.1
	github.com/sclevine/yj v0.0.0-20190506050358-d9a48607cc5c
	github.com/spf13/cast v1.3.1 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/tdakkota/asciicheck v0.0.0-20200416200610-e657995f937b // indirect
	github.com/timakin/bodyclose v0.0.0-20200424151742-cb6215831a94 // indirect
	golang.org/x/tools v0.0.0-20200916195026-c9a70fc28ce3
	gopkg.in/ini.v1 v1.56.0 // indirect
	mvdan.cc/unparam v0.0.0-20200501210554-b37ab49443f7 // indirect
	sourcegraph.com/sqs/pbtypes v1.0.0 // indirect
)

replace github.com/buildpacks/lifecycle v0.8.1 => ../
