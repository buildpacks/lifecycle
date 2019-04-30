# Go parameters
GOCMD=go
GOENV=GO111MODULE=on GOOS=linux GOARCH=amd64 CGO_ENABLED=0
GOBUILD=$(GOCMD) build -mod=vendor
GOTEST=$(GOCMD) test -mod=vendor
LIFECYCLE_VERSION?=dev
ARCHIVE_NAME=lifecycle-$(LIFECYCLE_VERSION)+linux.x86-64

all: test build package
build:
	mkdir -p ./out/$(ARCHIVE_NAME)
	$(GOENV) $(GOBUILD) -o ./out/$(ARCHIVE_NAME)/detector -a ./cmd/detector/main.go
	$(GOENV) $(GOBUILD) -o ./out/$(ARCHIVE_NAME)/restorer -a ./cmd/restorer/main.go
	$(GOENV) $(GOBUILD) -o ./out/$(ARCHIVE_NAME)/analyzer -a ./cmd/analyzer/main.go
	$(GOENV) $(GOBUILD) -o ./out/$(ARCHIVE_NAME)/builder -a ./cmd/builder/main.go
	$(GOENV) $(GOBUILD) -o ./out/$(ARCHIVE_NAME)/exporter -a ./cmd/exporter/main.go
	$(GOENV) $(GOBUILD) -o ./out/$(ARCHIVE_NAME)/cacher -a ./cmd/cacher/main.go
	$(GOENV) $(GOBUILD) -o ./out/$(ARCHIVE_NAME)/launcher -a ./cmd/launcher/main.go

format:
	test -z $$($(GOCMD) fmt ./...)

vet:
	$(GOCMD) vet $$($(GOCMD) list ./... | grep -v /testdata/)

test: format vet
	$(GOTEST) -v ./...

clean:
	rm -rf ./out

package:
	tar czf ./out/$(ARCHIVE_NAME).tgz -C out $(ARCHIVE_NAME)