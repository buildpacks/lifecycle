export GO111MODULE = on

GOCMD?=go
GOENV=GOOS=linux GOARCH=amd64 CGO_ENABLED=0
GOBUILD=$(GOCMD) build -mod=vendor -ldflags "-X 'github.com/buildpack/lifecycle.Version=$(LIFECYCLE_VERSION)' -X 'github.com/buildpack/lifecycle.SCMRepository=$(SCM_REPO)' -X 'github.com/buildpack/lifecycle.SCMCommit=$(SCM_COMMIT)'"
GOTEST=$(GOCMD) test -mod=vendor
LIFECYCLE_VERSION?=0.0.0
SCM_REPO?=
SCM_COMMIT=$$(git rev-parse --short HEAD)
ARCHIVE_NAME=lifecycle-v$(LIFECYCLE_VERSION)+linux.x86-64

all: test build package
build:
	@echo "> Building lifecycle..."
	mkdir -p ./out/$(ARCHIVE_NAME)
	$(GOENV) $(GOBUILD) -o ./out/$(ARCHIVE_NAME)/detector -a ./cmd/detector
	$(GOENV) $(GOBUILD) -o ./out/$(ARCHIVE_NAME)/restorer -a ./cmd/restorer
	$(GOENV) $(GOBUILD) -o ./out/$(ARCHIVE_NAME)/analyzer -a ./cmd/analyzer
	$(GOENV) $(GOBUILD) -o ./out/$(ARCHIVE_NAME)/builder -a ./cmd/builder
	$(GOENV) $(GOBUILD) -o ./out/$(ARCHIVE_NAME)/exporter -a ./cmd/exporter
	$(GOENV) $(GOBUILD) -o ./out/$(ARCHIVE_NAME)/cacher -a ./cmd/cacher
	$(GOENV) $(GOBUILD) -o ./out/$(ARCHIVE_NAME)/launcher -a ./cmd/launcher

install-goimports:
	@echo "> Installing goimports..."
	$(GOCMD) install -mod=vendor golang.org/x/tools/cmd/goimports

install-yj:
	@echo "> Installing yj..."
	$(GOCMD) install -mod=vendor github.com/sclevine/yj

format: install-goimports
	@echo "> Formating code..."
	test -z $$(goimports -l -w -local github.com/buildpack/lifecycle $$(find . -type f -name '*.go' -not -path "./vendor/*"))

vet:
	@echo "> Vetting code..."
	$(GOCMD) vet -mod=vendor $$($(GOCMD) list -mod=vendor ./... | grep -v /testdata/)

test: unit acceptance

unit: format vet install-yj
	@echo "> Running unit tests..."
	$(GOTEST) -v -count=1 ./...

acceptance: format vet
	@echo "> Running acceptance tests..."
	$(GOTEST) -v -count=1 -tags=acceptance ./acceptance/...

clean:
	@echo "> Cleaning workspace..."
	rm -rf ./out

package:
	@echo "> Packaging lifecycle..."
	tar czf ./out/$(ARCHIVE_NAME).tgz -C out $(ARCHIVE_NAME)