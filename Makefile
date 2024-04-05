ifeq ($(OS),Windows_NT)
SHELL:=cmd.exe
PWD?=$(subst /,\,${CURDIR})
LDFLAGS=-s -w
BLANK:=
/:=\$(BLANK)
else
/:=/
endif

PARSED_COMMIT:=$(shell git rev-parse --short HEAD)

ifeq ($(LIFECYCLE_VERSION),)
LIFECYCLE_VERSION:=$(shell go run tools/version/main.go)
LIFECYCLE_IMAGE_TAG?=$(PARSED_COMMIT)
else
LIFECYCLE_IMAGE_TAG?=$(LIFECYCLE_VERSION)
endif

ACCEPTANCE_TIMEOUT?=2400s
GOCMD?=go
GOENV=GOARCH=$(GOARCH) CGO_ENABLED=0
LIFECYCLE_DESCRIPTOR_PATH?=lifecycle.toml
SCM_REPO?=github.com/buildpacks/lifecycle
SCM_COMMIT?=$(PARSED_COMMIT)
LDFLAGS=-s -w
LDFLAGS+=-X 'github.com/buildpacks/lifecycle/cmd.SCMRepository=$(SCM_REPO)'
LDFLAGS+=-X 'github.com/buildpacks/lifecycle/cmd.SCMCommit=$(SCM_COMMIT)'
LDFLAGS+=-X 'github.com/buildpacks/lifecycle/cmd.Version=$(LIFECYCLE_VERSION)'
GOBUILD:=go build $(GOFLAGS) -ldflags "$(LDFLAGS)"
GOTEST=$(GOCMD) test $(GOFLAGS)
BUILD_DIR?=$(PWD)$/out
WINDOWS_COMPILATION_IMAGE?=golang:1.22-windowsservercore-1809
SOURCE_COMPILATION_IMAGE?=lifecycle-img
BUILD_CTR?=lifecycle-ctr
DOCKER_CMD?=make test

GOFILES := $(shell $(GOCMD) run tools$/lister$/main.go)

all: test build package

build: build-linux-amd64 build-linux-arm64 build-windows-amd64 build-linux-ppc64le build-linux-s390x

build-linux-amd64: build-linux-amd64-lifecycle build-linux-amd64-symlinks build-linux-amd64-launcher
build-linux-arm64: build-linux-arm64-lifecycle build-linux-arm64-symlinks build-linux-arm64-launcher
build-windows-amd64: build-windows-amd64-lifecycle build-windows-amd64-symlinks build-windows-amd64-launcher
build-linux-ppc64le: build-linux-ppc64le-lifecycle build-linux-ppc64le-symlinks build-linux-ppc64le-launcher
build-linux-s390x: build-linux-s390x-lifecycle build-linux-s390x-symlinks build-linux-s390x-launcher

build-image-linux-amd64: build-linux-amd64 package-linux-amd64
build-image-linux-amd64: ARCHIVE_PATH=$(BUILD_DIR)/lifecycle-v$(LIFECYCLE_VERSION)+linux.x86-64.tgz
build-image-linux-amd64:
	$(GOCMD) run ./tools/image/main.go -daemon -lifecyclePath $(ARCHIVE_PATH) -os linux -arch amd64 -tag lifecycle:$(LIFECYCLE_IMAGE_TAG)

build-image-linux-arm64: build-linux-arm64 package-linux-arm64
build-image-linux-arm64: ARCHIVE_PATH=$(BUILD_DIR)/lifecycle-v$(LIFECYCLE_VERSION)+linux.arm64.tgz
build-image-linux-arm64:
	$(GOCMD) run ./tools/image/main.go -daemon -lifecyclePath $(ARCHIVE_PATH) -os linux -arch arm64 -tag lifecycle:$(LIFECYCLE_IMAGE_TAG)

build-image-windows-amd64: build-windows-amd64 package-windows-amd64
build-image-windows-amd64: ARCHIVE_PATH=$(BUILD_DIR)/lifecycle-v$(LIFECYCLE_VERSION)+windows.x86-64.tgz
build-image-windows-amd64:
	$(GOCMD) run ./tools/image/main.go -daemon -lifecyclePath $(ARCHIVE_PATH) -os windows -arch amd64 -tag lifecycle:$(LIFECYCLE_IMAGE_TAG)

build-image-linux-ppc64le: build-linux-ppc64le package-linux-ppc64le
build-image-linux-ppc64le: ARCHIVE_PATH=$(BUILD_DIR)/lifecycle-v$(LIFECYCLE_VERSION)+linux.ppc64le.tgz
build-image-linux-ppc64le:
	$(GOCMD) run ./tools/image/main.go -daemon -lifecyclePath $(ARCHIVE_PATH) -os linux -arch ppc64le -tag lifecycle:$(LIFECYCLE_IMAGE_TAG)

build-image-linux-s390x: build-linux-s390x package-linux-s390x
build-image-linux-s390x: ARCHIVE_PATH=$(BUILD_DIR)/lifecycle-v$(LIFECYCLE_VERSION)+linux.s390x.tgz
build-image-linux-s390x:
	$(GOCMD) run ./tools/image/main.go -daemon -lifecyclePath $(ARCHIVE_PATH) -os linux -arch s390x -tag lifecycle:$(LIFECYCLE_IMAGE_TAG)

build-linux-amd64-lifecycle: $(BUILD_DIR)/linux-amd64/lifecycle/lifecycle

build-linux-arm64-lifecycle: $(BUILD_DIR)/linux-arm64/lifecycle/lifecycle

build-linux-ppc64le-lifecycle: $(BUILD_DIR)/linux-ppc64le/lifecycle/lifecycle

build-linux-s390x-lifecycle: $(BUILD_DIR)/linux-s390x/lifecycle/lifecycle

$(BUILD_DIR)/linux-amd64/lifecycle/lifecycle: export GOOS:=linux
$(BUILD_DIR)/linux-amd64/lifecycle/lifecycle: export GOARCH:=amd64
$(BUILD_DIR)/linux-amd64/lifecycle/lifecycle: OUT_DIR?=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle
$(BUILD_DIR)/linux-amd64/lifecycle/lifecycle: $(GOFILES)
$(BUILD_DIR)/linux-amd64/lifecycle/lifecycle:
	@echo "> Building lifecycle/lifecycle for $(GOOS)/$(GOARCH)..."
	mkdir -p $(OUT_DIR)
	$(GOENV) $(GOBUILD) -o $(OUT_DIR)/lifecycle -a ./cmd/lifecycle

$(BUILD_DIR)/linux-arm64/lifecycle/lifecycle: export GOOS:=linux
$(BUILD_DIR)/linux-arm64/lifecycle/lifecycle: export GOARCH:=arm64
$(BUILD_DIR)/linux-arm64/lifecycle/lifecycle: OUT_DIR?=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle
$(BUILD_DIR)/linux-arm64/lifecycle/lifecycle: $(GOFILES)
$(BUILD_DIR)/linux-arm64/lifecycle/lifecycle:
	@echo "> Building lifecycle/lifecycle for $(GOOS)/$(GOARCH)..."
	mkdir -p $(OUT_DIR)
	$(GOENV) $(GOBUILD) -o $(OUT_DIR)/lifecycle -a ./cmd/lifecycle

$(BUILD_DIR)/linux-ppc64le/lifecycle/lifecycle: export GOOS:=linux
$(BUILD_DIR)/linux-ppc64le/lifecycle/lifecycle: export GOARCH:=ppc64le
$(BUILD_DIR)/linux-ppc64le/lifecycle/lifecycle: OUT_DIR?=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle
$(BUILD_DIR)/linux-ppc64le/lifecycle/lifecycle: $(GOFILES)
$(BUILD_DIR)/linux-ppc64le/lifecycle/lifecycle:
	@echo "> Building lifecycle/lifecycle for $(GOOS)/$(GOARCH)..."
	mkdir -p $(OUT_DIR)
	$(GOENV) $(GOBUILD) -o $(OUT_DIR)/lifecycle -a ./cmd/lifecycle

$(BUILD_DIR)/linux-s390x/lifecycle/lifecycle: export GOOS:=linux
$(BUILD_DIR)/linux-s390x/lifecycle/lifecycle: export GOARCH:=s390x
$(BUILD_DIR)/linux-s390x/lifecycle/lifecycle: OUT_DIR?=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle
$(BUILD_DIR)/linux-s390x/lifecycle/lifecycle: $(GOFILES)
$(BUILD_DIR)/linux-s390x/lifecycle/lifecycle:
	@echo "> Building lifecycle/lifecycle for $(GOOS)/$(GOARCH)..."
	mkdir -p $(OUT_DIR)
	$(GOENV) $(GOBUILD) -o $(OUT_DIR)/lifecycle -a ./cmd/lifecycle

build-linux-amd64-launcher: $(BUILD_DIR)/linux-amd64/lifecycle/launcher

$(BUILD_DIR)/linux-amd64/lifecycle/launcher: export GOOS:=linux
$(BUILD_DIR)/linux-amd64/lifecycle/launcher: export GOARCH:=amd64
$(BUILD_DIR)/linux-amd64/lifecycle/launcher: OUT_DIR?=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle
$(BUILD_DIR)/linux-amd64/lifecycle/launcher: $(GOFILES)
$(BUILD_DIR)/linux-amd64/lifecycle/launcher:
	@echo "> Building lifecycle/launcher for $(GOOS)/$(GOARCH)..."
	mkdir -p $(OUT_DIR)
	$(GOENV) $(GOBUILD) -o $(OUT_DIR)/launcher -a ./cmd/launcher
	test $$(du -m $(OUT_DIR)/launcher|cut -f 1) -le 3

build-linux-arm64-launcher: $(BUILD_DIR)/linux-arm64/lifecycle/launcher

$(BUILD_DIR)/linux-arm64/lifecycle/launcher: export GOOS:=linux
$(BUILD_DIR)/linux-arm64/lifecycle/launcher: export GOARCH:=arm64
$(BUILD_DIR)/linux-arm64/lifecycle/launcher: OUT_DIR?=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle
$(BUILD_DIR)/linux-arm64/lifecycle/launcher: $(GOFILES)
$(BUILD_DIR)/linux-arm64/lifecycle/launcher:
	@echo "> Building lifecycle/launcher for $(GOOS)/$(GOARCH)..."
	mkdir -p $(OUT_DIR)
	$(GOENV) $(GOBUILD) -o $(OUT_DIR)/launcher -a ./cmd/launcher
	test $$(du -m $(OUT_DIR)/launcher|cut -f 1) -le 3

build-linux-ppc64le-launcher: $(BUILD_DIR)/linux-ppc64le/lifecycle/launcher

$(BUILD_DIR)/linux-ppc64le/lifecycle/launcher: export GOOS:=linux
$(BUILD_DIR)/linux-ppc64le/lifecycle/launcher: export GOARCH:=ppc64le
$(BUILD_DIR)/linux-ppc64le/lifecycle/launcher: OUT_DIR?=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle
$(BUILD_DIR)/linux-ppc64le/lifecycle/launcher: $(GOFILES)
$(BUILD_DIR)/linux-ppc64le/lifecycle/launcher:
	@echo "> Building lifecycle/launcher for $(GOOS)/$(GOARCH)..."
	mkdir -p $(OUT_DIR)
	$(GOENV) $(GOBUILD) -o $(OUT_DIR)/launcher -a ./cmd/launcher
	test $$(du -m $(OUT_DIR)/launcher|cut -f 1) -le 3

build-linux-s390x-launcher: $(BUILD_DIR)/linux-s390x/lifecycle/launcher

$(BUILD_DIR)/linux-s390x/lifecycle/launcher: export GOOS:=linux
$(BUILD_DIR)/linux-s390x/lifecycle/launcher: export GOARCH:=s390x
$(BUILD_DIR)/linux-s390x/lifecycle/launcher: OUT_DIR?=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle
$(BUILD_DIR)/linux-s390x/lifecycle/launcher: $(GOFILES)
$(BUILD_DIR)/linux-s390x/lifecycle/launcher:
	@echo "> Building lifecycle/launcher for $(GOOS)/$(GOARCH)..."
	mkdir -p $(OUT_DIR)
	$(GOENV) $(GOBUILD) -o $(OUT_DIR)/launcher -a ./cmd/launcher
	test $$(du -m $(OUT_DIR)/launcher|cut -f 1) -le 3

build-linux-amd64-symlinks: export GOOS:=linux
build-linux-amd64-symlinks: export GOARCH:=amd64
build-linux-amd64-symlinks: OUT_DIR?=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle
build-linux-amd64-symlinks:
	@echo "> Creating phase symlinks for $(GOOS)/$(GOARCH)..."
	ln -sf lifecycle $(OUT_DIR)/detector
	ln -sf lifecycle $(OUT_DIR)/analyzer
	ln -sf lifecycle $(OUT_DIR)/restorer
	ln -sf lifecycle $(OUT_DIR)/builder
	ln -sf lifecycle $(OUT_DIR)/exporter
	ln -sf lifecycle $(OUT_DIR)/rebaser
	ln -sf lifecycle $(OUT_DIR)/creator
	ln -sf lifecycle $(OUT_DIR)/extender

build-linux-arm64-symlinks: export GOOS:=linux
build-linux-arm64-symlinks: export GOARCH:=arm64
build-linux-arm64-symlinks: OUT_DIR?=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle
build-linux-arm64-symlinks:
	@echo "> Creating phase symlinks for $(GOOS)/$(GOARCH)..."
	ln -sf lifecycle $(OUT_DIR)/detector
	ln -sf lifecycle $(OUT_DIR)/analyzer
	ln -sf lifecycle $(OUT_DIR)/restorer
	ln -sf lifecycle $(OUT_DIR)/builder
	ln -sf lifecycle $(OUT_DIR)/exporter
	ln -sf lifecycle $(OUT_DIR)/rebaser
	ln -sf lifecycle $(OUT_DIR)/creator
	ln -sf lifecycle $(OUT_DIR)/extender

build-linux-ppc64le-symlinks: export GOOS:=linux
build-linux-ppc64le-symlinks: export GOARCH:=ppc64le
build-linux-ppc64le-symlinks: OUT_DIR?=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle
build-linux-ppc64le-symlinks:
	@echo "> Creating phase symlinks for $(GOOS)/$(GOARCH)..."
	ln -sf lifecycle $(OUT_DIR)/detector
	ln -sf lifecycle $(OUT_DIR)/analyzer
	ln -sf lifecycle $(OUT_DIR)/restorer
	ln -sf lifecycle $(OUT_DIR)/builder
	ln -sf lifecycle $(OUT_DIR)/exporter
	ln -sf lifecycle $(OUT_DIR)/rebaser
	ln -sf lifecycle $(OUT_DIR)/creator
	ln -sf lifecycle $(OUT_DIR)/extender

build-linux-s390x-symlinks: export GOOS:=linux
build-linux-s390x-symlinks: export GOARCH:=s390x
build-linux-s390x-symlinks: OUT_DIR?=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle
build-linux-s390x-symlinks:
	@echo "> Creating phase symlinks for $(GOOS)/$(GOARCH)..."
	ln -sf lifecycle $(OUT_DIR)/detector
	ln -sf lifecycle $(OUT_DIR)/analyzer
	ln -sf lifecycle $(OUT_DIR)/restorer
	ln -sf lifecycle $(OUT_DIR)/builder
	ln -sf lifecycle $(OUT_DIR)/exporter
	ln -sf lifecycle $(OUT_DIR)/rebaser
	ln -sf lifecycle $(OUT_DIR)/creator
	ln -sf lifecycle $(OUT_DIR)/extender

build-windows-amd64-lifecycle: $(BUILD_DIR)/windows-amd64/lifecycle/lifecycle.exe

$(BUILD_DIR)/windows-amd64/lifecycle/lifecycle.exe: export GOOS:=windows
$(BUILD_DIR)/windows-amd64/lifecycle/lifecycle.exe: export GOARCH:=amd64
$(BUILD_DIR)/windows-amd64/lifecycle/lifecycle.exe: OUT_DIR?=$(BUILD_DIR)$/$(GOOS)-$(GOARCH)$/lifecycle
$(BUILD_DIR)/windows-amd64/lifecycle/lifecycle.exe: $(GOFILES)
$(BUILD_DIR)/windows-amd64/lifecycle/lifecycle.exe:
	@echo "> Building lifecycle/lifecycle for $(GOOS)/$(GOARCH)..."
	$(GOBUILD) -o $(OUT_DIR)$/lifecycle.exe -a .$/cmd$/lifecycle

build-windows-amd64-launcher: $(BUILD_DIR)/windows-amd64/lifecycle/launcher.exe

$(BUILD_DIR)/windows-amd64/lifecycle/launcher.exe: export GOOS:=windows
$(BUILD_DIR)/windows-amd64/lifecycle/launcher.exe: export GOARCH:=amd64
$(BUILD_DIR)/windows-amd64/lifecycle/launcher.exe: OUT_DIR?=$(BUILD_DIR)$/$(GOOS)-$(GOARCH)$/lifecycle
$(BUILD_DIR)/windows-amd64/lifecycle/launcher.exe: $(GOFILES)
$(BUILD_DIR)/windows-amd64/lifecycle/launcher.exe:
	@echo "> Building lifecycle/launcher for $(GOOS)/$(GOARCH)..."
	$(GOBUILD) -o $(OUT_DIR)$/launcher.exe -a .$/cmd$/launcher

build-windows-amd64-symlinks: export GOOS:=windows
build-windows-amd64-symlinks: export GOARCH:=amd64
build-windows-amd64-symlinks: OUT_DIR?=$(BUILD_DIR)$/$(GOOS)-$(GOARCH)$/lifecycle
build-windows-amd64-symlinks:
	@echo "> Creating phase symlinks for Windows..."
ifeq ($(OS),Windows_NT)
	call del $(OUT_DIR)$/detector.exe
	call del $(OUT_DIR)$/analyzer.exe
	call del $(OUT_DIR)$/restorer.exe
	call del $(OUT_DIR)$/builder.exe
	call del $(OUT_DIR)$/exporter.exe
	call del $(OUT_DIR)$/rebaser.exe
	call del $(OUT_DIR)$/creator.exe
	call mklink $(OUT_DIR)$/detector.exe lifecycle.exe
	call mklink $(OUT_DIR)$/analyzer.exe lifecycle.exe
	call mklink $(OUT_DIR)$/restorer.exe lifecycle.exe
	call mklink $(OUT_DIR)$/builder.exe  lifecycle.exe
	call mklink $(OUT_DIR)$/exporter.exe lifecycle.exe
	call mklink $(OUT_DIR)$/rebaser.exe  lifecycle.exe
	call mklink $(OUT_DIR)$/creator.exe  lifecycle.exe
else
	ln -sf lifecycle.exe $(OUT_DIR)$/detector.exe
	ln -sf lifecycle.exe $(OUT_DIR)$/analyzer.exe
	ln -sf lifecycle.exe $(OUT_DIR)$/restorer.exe
	ln -sf lifecycle.exe $(OUT_DIR)$/builder.exe
	ln -sf lifecycle.exe $(OUT_DIR)$/exporter.exe
	ln -sf lifecycle.exe $(OUT_DIR)$/rebaser.exe
	ln -sf lifecycle.exe $(OUT_DIR)$/creator.exe
endif

## DARWIN ARM64/AMD64
include lifecycle.mk
include launcher.mk
build-darwin-arm64: build-darwin-arm64-lifecycle build-darwin-arm64-launcher
build-darwin-arm64-lifecycle:
	$(eval GOARCH := arm64)
	$(eval TARGET := darwin-arm64)
	$(eval OUT_DIR := $(BUILD_DIR)/$(TARGET)/lifecycle)
	$(call build_lifecycle)
build-darwin-arm64-launcher:
	$(eval GOARCH := arm64)
	$(eval TARGET := darwin-arm64)
	$(eval OUT_DIR := $(BUILD_DIR)/$(TARGET)/lifecycle)
	$(call build_launcher)

build-darwin-amd64: build-darwin-amd64-lifecycle build-darwin-amd64-launcher
build-darwin-amd64-lifecycle:
	$(eval GOARCH := amd64)
	$(eval TARGET := darwin-amd64)
	$(eval OUT_DIR := $(BUILD_DIR)/$(TARGET)/lifecycle)
	$(call build_lifecycle)
build-darwin-amd64-launcher:
	$(eval GOARCH := amd64)
	$(eval TARGET := darwin-amd64)
	$(eval OUT_DIR := $(BUILD_DIR)/$(TARGET)/lifecycle)
	$(call build_launcher)

generate-sbom: run-syft-windows run-syft-linux-amd64 run-syft-linux-arm64 run-syft-linux-ppc64le run-syft-linux-s390x

run-syft-windows: install-syft
run-syft-windows: export GOOS:=windows
run-syft-windows: export GOARCH:=amd64
run-syft-windows:
	@echo "> Running syft..."
	syft $(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/lifecycle.exe -o json=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/lifecycle.sbom.syft.json -o spdx-json=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/lifecycle.sbom.spdx.json -o cyclonedx-json=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/lifecycle.sbom.cdx.json
	syft $(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/launcher.exe -o json=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/launcher.sbom.syft.json -o spdx-json=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/launcher.sbom.spdx.json -o cyclonedx-json=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/launcher.sbom.cdx.json

run-syft-linux-amd64: install-syft
run-syft-linux-amd64: export GOOS:=linux
run-syft-linux-amd64: export GOARCH:=amd64
run-syft-linux-amd64:
	@echo "> Running syft..."
	syft $(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/lifecycle -o json=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/lifecycle.sbom.syft.json -o spdx-json=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/lifecycle.sbom.spdx.json -o cyclonedx-json=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/lifecycle.sbom.cdx.json
	syft $(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/launcher -o json=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/launcher.sbom.syft.json -o spdx-json=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/launcher.sbom.spdx.json -o cyclonedx-json=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/launcher.sbom.cdx.json

run-syft-linux-arm64: install-syft
run-syft-linux-arm64: export GOOS:=linux
run-syft-linux-arm64: export GOARCH:=arm64
run-syft-linux-arm64:
	@echo "> Running syft..."
	syft $(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/lifecycle -o json=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/lifecycle.sbom.syft.json -o spdx-json=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/lifecycle.sbom.spdx.json -o cyclonedx-json=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/lifecycle.sbom.cdx.json
	syft $(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/launcher -o json=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/launcher.sbom.syft.json -o spdx-json=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/launcher.sbom.spdx.json -o cyclonedx-json=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/launcher.sbom.cdx.json

run-syft-linux-ppc64le: install-syft
run-syft-linux-ppc64le: export GOOS:=linux
run-syft-linux-ppc64le: export GOARCH:=ppc64le
run-syft-linux-ppc64le:
	@echo "> Running syft..."
	syft $(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/lifecycle -o json=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/lifecycle.sbom.syft.json -o spdx-json=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/lifecycle.sbom.spdx.json -o cyclonedx-json=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/lifecycle.sbom.cdx.json
	syft $(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/launcher -o json=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/launcher.sbom.syft.json -o spdx-json=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/launcher.sbom.spdx.json -o cyclonedx-json=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/launcher.sbom.cdx.json

run-syft-linux-s390x: install-syft
run-syft-linux-s390x: export GOOS:=linux
run-syft-linux-s390x: export GOARCH:=s390x
run-syft-linux-s390x:
	@echo "> Running syft..."
	syft $(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/lifecycle -o json=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/lifecycle.sbom.syft.json -o spdx-json=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/lifecycle.sbom.spdx.json -o cyclonedx-json=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/lifecycle.sbom.cdx.json
	syft $(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/launcher -o json=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/launcher.sbom.syft.json -o spdx-json=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/launcher.sbom.spdx.json -o cyclonedx-json=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle/launcher.sbom.cdx.json

install-syft:
	@echo "> Installing syft..."
	curl -sSfL https://raw.githubusercontent.com/anchore/syft/main/install.sh | sh -s -- -b /usr/local/bin

install-goimports:
	@echo "> Installing goimports..."
	$(GOCMD) install golang.org/x/tools/cmd/goimports@v0.1.2

install-yj:
	@echo "> Installing yj..."
	$(GOCMD) install github.com/sclevine/yj@v0.0.0-20210612025309-737bdf40a5d1

install-mockgen:
	@echo "> Installing mockgen..."
	$(GOCMD) install github.com/golang/mock/mockgen@v1.5.0

install-golangci-lint:
	@echo "> Installing golangci-lint..."
	$(GOCMD) install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.57.2

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

tidy:
	@echo "> Tidying modules..."
	$(GOCMD) mod tidy

test: unit acceptance

# append coverage arguments
ifeq ($(TEST_COVERAGE), 1)
unit: GOTESTFLAGS:=$(GOTESTFLAGS) -coverprofile=./out/tests/coverage-unit.txt -covermode=atomic
endif
unit: out
unit: UNIT_PACKAGES=$(shell $(GOCMD) list ./... | grep -v acceptance)
unit: format lint tidy install-yj
	@echo "> Running unit tests..."
	$(GOTEST) $(GOTESTFLAGS) -v -count=1 $(UNIT_PACKAGES)

out:
	@mkdir out || (exit 0)
	mkdir out$/tests || (exit 0)

acceptance: format tidy
	@echo "> Running acceptance tests..."
	$(GOTEST) -v -count=1 -tags=acceptance -timeout=$(ACCEPTANCE_TIMEOUT) ./acceptance/...

clean:
	@echo "> Cleaning workspace..."
	rm -rf $(BUILD_DIR)

package:  generate-sbom package-linux-amd64 package-linux-arm64 package-windows-amd64 package-linux-ppc64le package-linux-s390x

package-linux-amd64: GOOS:=linux
package-linux-amd64: GOARCH:=amd64
package-linux-amd64: INPUT_DIR:=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle
package-linux-amd64: ARCHIVE_PATH=$(BUILD_DIR)/lifecycle-v$(LIFECYCLE_VERSION)+$(GOOS).x86-64.tgz
package-linux-amd64: PACKAGER=./tools/packager/main.go
package-linux-amd64:
	@echo "> Packaging lifecycle for $(GOOS)/$(GOARCH)..."
	$(GOCMD) run $(PACKAGER) --inputDir $(INPUT_DIR) -archivePath $(ARCHIVE_PATH) -descriptorPath $(LIFECYCLE_DESCRIPTOR_PATH) -version $(LIFECYCLE_VERSION)

package-linux-arm64: GOOS:=linux
package-linux-arm64: GOARCH:=arm64
package-linux-arm64: INPUT_DIR:=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle
package-linux-arm64: ARCHIVE_PATH=$(BUILD_DIR)/lifecycle-v$(LIFECYCLE_VERSION)+$(GOOS).arm64.tgz
package-linux-arm64: PACKAGER=./tools/packager/main.go
package-linux-arm64:
	@echo "> Packaging lifecycle for $(GOOS)/$(GOARCH)..."
	$(GOCMD) run $(PACKAGER) --inputDir $(INPUT_DIR) -archivePath $(ARCHIVE_PATH) -descriptorPath $(LIFECYCLE_DESCRIPTOR_PATH) -version $(LIFECYCLE_VERSION)

package-linux-ppc64le: GOOS:=linux
package-linux-ppc64le: GOARCH:=ppc64le
package-linux-ppc64le: INPUT_DIR:=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle
package-linux-ppc64le: ARCHIVE_PATH=$(BUILD_DIR)/lifecycle-v$(LIFECYCLE_VERSION)+$(GOOS).ppc64le.tgz
package-linux-ppc64le: PACKAGER=./tools/packager/main.go
package-linux-ppc64le:
	@echo "> Packaging lifecycle for $(GOOS)/$(GOARCH)..."
	$(GOCMD) run $(PACKAGER) --inputDir $(INPUT_DIR) -archivePath $(ARCHIVE_PATH) -descriptorPath $(LIFECYCLE_DESCRIPTOR_PATH) -version $(LIFECYCLE_VERSION)

package-linux-s390x: GOOS:=linux
package-linux-s390x: GOARCH:=s390x
package-linux-s390x: INPUT_DIR:=$(BUILD_DIR)/$(GOOS)-$(GOARCH)/lifecycle
package-linux-s390x: ARCHIVE_PATH=$(BUILD_DIR)/lifecycle-v$(LIFECYCLE_VERSION)+$(GOOS).s390x.tgz
package-linux-s390x: PACKAGER=./tools/packager/main.go
package-linux-s390x:
	@echo "> Packaging lifecycle for $(GOOS)/$(GOARCH)..."
	$(GOCMD) run $(PACKAGER) --inputDir $(INPUT_DIR) -archivePath $(ARCHIVE_PATH) -descriptorPath $(LIFECYCLE_DESCRIPTOR_PATH) -version $(LIFECYCLE_VERSION)

package-windows-amd64: GOOS:=windows
package-windows-amd64: GOARCH:=amd64
package-windows-amd64: INPUT_DIR:=$(BUILD_DIR)$/$(GOOS)-$(GOARCH)$/lifecycle
package-windows-amd64: ARCHIVE_PATH=$(BUILD_DIR)$/lifecycle-v$(LIFECYCLE_VERSION)+$(GOOS).x86-64.tgz
package-windows-amd64: PACKAGER=.$/tools$/packager$/main.go
package-windows-amd64:
	@echo "> Packaging lifecycle for $(GOOS)/$(GOARCH)..."
	$(GOCMD) run $(PACKAGER) --inputDir $(INPUT_DIR) -archivePath $(ARCHIVE_PATH) -descriptorPath $(LIFECYCLE_DESCRIPTOR_PATH) -version $(LIFECYCLE_VERSION)

# Ensure workdir is clean and build image from .git
docker-build-source-image-windows: $(GOFILES)
docker-build-source-image-windows:
	$(if $(shell git status --short), @echo Uncommitted changes. Refusing to run. && exit 1)
	docker build .git -f tools/Dockerfile.windows --tag $(SOURCE_COMPILATION_IMAGE) --build-arg image_tag=$(WINDOWS_COMPILATION_IMAGE) --cache-from=$(SOURCE_COMPILATION_IMAGE) --isolation=process --compress

docker-run-windows: docker-build-source-image-windows
docker-run-windows:
	@echo "> Running '$(DOCKER_CMD)' in docker windows..."
	@docker volume rm -f lifecycle-out
	docker run -v lifecycle-out:c:/lifecycle/out -e LIFECYCLE_VERSION -e PLATFORM_API -e BUILDPACK_API -v gopathcache:c:/gopath -v '\\.\pipe\docker_engine:\\.\pipe\docker_engine' --isolation=process --interactive --tty --rm $(SOURCE_COMPILATION_IMAGE) $(DOCKER_CMD)
	docker run -v lifecycle-out:c:/lifecycle/out --rm $(SOURCE_COMPILATION_IMAGE) tar -cf- out | tar -xf-
	@docker volume rm -f lifecycle-out
