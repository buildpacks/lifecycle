module github.com/buildpacks/lifecycle/tools

go 1.14

require (
	github.com/buildpacks/imgutil v0.0.0-20200831154319-afd98bd2f655
	github.com/buildpacks/lifecycle v0.8.1
	github.com/golang/mock v1.4.4
	github.com/golangci/golangci-lint v1.22.2
	github.com/sclevine/yj v0.0.0-20190506050358-d9a48607cc5c
	golang.org/x/tools v0.0.0-20200210192313-1ace956b0e17
	rsc.io/quote/v3 v3.1.0 // indirect
)

replace github.com/buildpacks/lifecycle v0.8.1 => ../
