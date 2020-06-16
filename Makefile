ifeq ($(OS),Windows_NT)
SHELL:=cmd.exe
PWD?=$(subst /,\,${CURDIR})
LDFLAGS=-s -w
BLANK:=
/:=\$(BLANK)
LIFECYCLE_VERSION?=$(shell type VERSION)
else
/:=/
LIFECYCLE_VERSION?=$(shell cat VERSION)
endif

GOCMD?=go
GOARCH?=amd64
GOENV=GOARCH=$(GOARCH) CGO_ENABLED=0
LDFLAGS+=-X 'github.com/buildpacks/lifecycle/cmd.Version=$(LIFECYCLE_VERSION)'
LDFLAGS+=-X 'github.com/buildpacks/lifecycle/cmd.SCMRepository=$(SCM_REPO)'
LDFLAGS+=-X 'github.com/buildpacks/lifecycle/cmd.SCMCommit=$(SCM_COMMIT)'
LDFLAGS+=-X 'github.com/buildpacks/lifecycle/cmd.PlatformAPI=$(PLATFORM_API)'
GOBUILD=go build $(GOFLAGS) -ldflags "$(LDFLAGS)"
GOTEST=$(GOCMD) test $(GOFLAGS)
PLATFORM_API?=0.3
BUILDPACK_API?=0.2
SCM_REPO?=github.com/buildpacks/lifecycle
PARSED_COMMIT:=$(shell git rev-parse --short HEAD)
SCM_COMMIT?=$(PARSED_COMMIT)
BUILD_DIR?=$(PWD)$/out
LINUX_COMPILATION_IMAGE?=golang:1.13-alpine
WINDOWS_COMPILATION_IMAGE?=golang:1.14-windowsservercore-1809
SOURCE_COMPILATION_IMAGE?=lifecycle-img
BUILD_CTR?=lifecycle-ctr
DOCKER_CMD?=make test

all: test build package

build: build-linux build-windows

build-linux: build-linux-lifecycle build-linux-symlinks build-linux-launcher

ifeq ($(OS),Windows_NT)
build-windows: build-windows-on-windows
else
build-windows: build-windows-on-posix
endif

build-linux-lifecycle: export GOOS:=linux
build-linux-lifecycle: OUT_DIR:=$(BUILD_DIR)/$(GOOS)/lifecycle
build-linux-lifecycle: GOENV:=GOARCH=$(GOARCH) CGO_ENABLED=1
build-linux-lifecycle: DOCKER_RUN=docker run --workdir=/lifecycle -v $(OUT_DIR):/out -v $(PWD):/lifecycle $(LINUX_COMPILATION_IMAGE)
build-linux-lifecycle:
	@echo "> Building lifecycle/lifecycle for linux..."
	mkdir -p $(OUT_DIR)
	$(DOCKER_RUN) sh -c 'apk add build-base && $(GOENV) $(GOBUILD) -o /out/lifecycle -a ./cmd/lifecycle'

build-linux-launcher: export GOOS:=linux
build-linux-launcher: OUT_DIR?=$(BUILD_DIR)/$(GOOS)/lifecycle
build-linux-launcher:
	@echo "> Building lifecycle/launcher for linux..."
	mkdir -p $(OUT_DIR)
	$(GOENV) $(GOBUILD) -o $(OUT_DIR)/launcher -a ./cmd/launcher
	test $$(du -m $(OUT_DIR)/launcher|cut -f 1) -le 4

build-linux-symlinks: export GOOS:=linux
build-linux-symlinks: OUT_DIR:=$(BUILD_DIR)/$(GOOS)/lifecycle
build-linux-symlinks:
	@echo "> Creating phase symlinks for linux..."
	ln -sf lifecycle $(OUT_DIR)/detector
	ln -sf lifecycle $(OUT_DIR)/analyzer
	ln -sf lifecycle $(OUT_DIR)/restorer
	ln -sf lifecycle $(OUT_DIR)/builder
	ln -sf lifecycle $(OUT_DIR)/exporter
	ln -sf lifecycle $(OUT_DIR)/rebaser
	ln -sf lifecycle $(OUT_DIR)/creator

build-windows-on-windows: export GOOS:=windows
build-windows-on-windows: OUT_DIR:=$(BUILD_DIR)\$(GOOS)\lifecycle
build-windows-on-windows:
	@echo "> Building for windows..."
	rmdir /Q /S $(OUT_DIR) 2>NUL || (exit 0)
	mkdir $(OUT_DIR)
	$(GOBUILD) -o $(OUT_DIR)\launcher.exe -a ./cmd/launcher
	$(GOBUILD) -o $(OUT_DIR)\lifecycle.exe -a ./cmd/lifecycle
	call mklink $(OUT_DIR)\detector.exe lifecycle.exe
	call mklink $(OUT_DIR)\analyzer.exe lifecycle.exe
	call mklink $(OUT_DIR)\restorer.exe lifecycle.exe
	call mklink $(OUT_DIR)\builder.exe  lifecycle.exe
	call mklink $(OUT_DIR)\exporter.exe lifecycle.exe
	call mklink $(OUT_DIR)\rebaser.exe  lifecycle.exe

build-windows-on-posix: export GOOS:=windows
build-windows-on-posix: OUT_DIR:=$(BUILD_DIR)/$(GOOS)/lifecycle
build-windows-on-posix:
	@echo "> Building for windows..."
	mkdir -p $(OUT_DIR)
	$(GOBUILD) -o $(OUT_DIR)/launcher.exe -a ./cmd/launcher
	test $$(du -m $(OUT_DIR)/launcher.exe|cut -f 1) -le 3
	$(GOBUILD) -o $(OUT_DIR)/lifecycle.exe -a ./cmd/lifecycle
	ln -sf lifecycle.exe $(OUT_DIR)/detector.exe
	ln -sf lifecycle.exe $(OUT_DIR)/analyzer.exe
	ln -sf lifecycle.exe $(OUT_DIR)/restorer.exe
	ln -sf lifecycle.exe $(OUT_DIR)/builder.exe
	ln -sf lifecycle.exe $(OUT_DIR)/exporter.exe
	ln -sf lifecycle.exe $(OUT_DIR)/rebaser.exe

build-darwin: export GOOS:=darwin
build-darwin: OUT_DIR:=$(BUILD_DIR)/$(GOOS)/lifecycle
build-darwin:
	@echo "> Building for macos..."
	mkdir -p $(OUT_DIR)
	$(GOENV) $(GOBUILD) -o $(OUT_DIR)/launcher -a ./cmd/launcher
	test $$(du -m $(OUT_DIR)/launcher|cut -f 1) -le 3
	$(GOENV) $(GOBUILD) -o $(OUT_DIR)/lifecycle -a ./cmd/lifecycle
	ln -sf lifecycle $(OUT_DIR)/detector
	ln -sf lifecycle $(OUT_DIR)/analyzer
	ln -sf lifecycle $(OUT_DIR)/restorer
	ln -sf lifecycle $(OUT_DIR)/builder
	ln -sf lifecycle $(OUT_DIR)/exporter
	ln -sf lifecycle $(OUT_DIR)/rebaser

install-goimports:
	@echo "> Installing goimports..."
	cd tools && $(GOCMD) install golang.org/x/tools/cmd/goimports

install-yj:
	@echo "> Installing yj..."
	cd tools && $(GOCMD) install github.com/sclevine/yj

install-mockgen:
	@echo "> Installing mockgen..."
	cd tools && $(GOCMD) install github.com/golang/mock/mockgen

install-golangci-lint:
	@echo "> Installing golangci-lint..."
	cd tools && $(GOCMD) install github.com/golangci/golangci-lint/cmd/golangci-lint

lint: install-golangci-lint
	@echo "> Linting code..."
	@golangci-lint run -c golangci.yaml

generate: install-mockgen
	@echo "> Generating..."
	$(GOCMD) generate
	$(GOCMD) generate ./launch

format: install-goimports
	@echo "> Formating code..."
	$(if $(shell goimports -l -w -local github.com/buildpacks/lifecycle .), @echo Fixed formatting errors. Re-run && exit 1)

test: unit acceptance

unit: format lint install-yj
	@echo "> Running unit tests..."
	$(GOTEST) -v -count=1 $$($(GOCMD) list ./... | grep -v acceptance)

acceptance: format lint
	@echo "> Running acceptance tests..."
	$(GOTEST) -v -count=1 -tags=acceptance ./acceptance/...

acceptance-darwin: format lint
	@echo "> Running acceptance tests..."
	$(GOTEST) -v -count=1 -tags=acceptance ./acceptance/...

clean:
	@echo "> Cleaning workspace..."
	rm -rf $(BUILD_DIR)

package: package-linux package-windows

package-linux: export LIFECYCLE_DESCRIPTOR:=$(LIFECYCLE_DESCRIPTOR)
package-linux: GOOS:=linux
package-linux: GOOS_DIR:=$(BUILD_DIR)/$(GOOS)
package-linux: ARCHIVE_NAME=lifecycle-v$(LIFECYCLE_VERSION)+$(GOOS).x86-64
package-linux:
	@echo "> Packaging lifecycle for $(GOOS)..."
	$(GOCMD) run tools/packager/main.go -os $(GOOS) -launcherExePath $(GOOS_DIR)/lifecycle/launcher -lifecycleExePath $(GOOS_DIR)/lifecycle/lifecycle -lifecycleVersion $(LIFECYCLE_VERSION) -platformAPI $(PLATFORM_API) -buildpackAPI $(BUILDPACK_API) -outputPackagePath $(BUILD_DIR)/$(ARCHIVE_NAME).tgz

package-windows: GOOS:=windows
package-windows: GOOS_DIR:=$(BUILD_DIR)\$(GOOS)
package-windows: ARCHIVE_NAME=lifecycle-v$(LIFECYCLE_VERSION)+$(GOOS).x86-64
package-windows:
	@echo "> Writing descriptor file for $(GOOS)..."
	$(GOCMD) run tools/packager/main.go -os $(GOOS) -launcherExePath $(GOOS_DIR)\lifecycle\launcher.exe -lifecycleExePath $(GOOS_DIR)\lifecycle\lifecycle.exe -lifecycleVersion $(LIFECYCLE_VERSION) -platformAPI $(PLATFORM_API) -buildpackAPI $(BUILDPACK_API) -outputPackagePath $(BUILD_DIR)\$(ARCHIVE_NAME).tgz

docker-build-source-image-windows:
	docker build -f tools/Dockerfile.windows --tag $(SOURCE_COMPILATION_IMAGE) --build-arg image_tag=$(WINDOWS_COMPILATION_IMAGE) --cache-from=$(SOURCE_COMPILATION_IMAGE) --isolation=process --quiet .

docker-run-windows: docker-build-source-image-windows
docker-run-windows:
	@echo "> Running '$(DOCKER_CMD)' in docker windows..."
	@docker volume rm -f lifecycle-out
	docker run -v lifecycle-out:c:/lifecycle/out -e LIFECYCLE_VERSION -e PLATFORM_API -e BUILDPACK_API -v gopathcache:c:/gopath -v '\\.\pipe\docker_engine:\\.\pipe\docker_engine' --isolation=process --rm $(SOURCE_COMPILATION_IMAGE) $(DOCKER_CMD)
	docker run -v lifecycle-out:c:/lifecycle/out --rm $(SOURCE_COMPILATION_IMAGE) tar -cf- out | tar -xf-
	@docker volume rm -f lifecycle-out

