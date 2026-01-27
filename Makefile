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
SOURCE_COMPILATION_IMAGE?=lifecycle-img
BUILD_CTR?=lifecycle-ctr
DOCKER_CMD?=make test

GOFILES := $(shell $(GOCMD) run tools$/lister$/main.go)

all: test build package

GOOS_ARCHS = linux/amd64 linux/arm64 linux/ppc64le linux/s390x darwin/amd64 darwin/arm64 freebsd/amd64 freebsd/arm64

build: build-linux-amd64 build-linux-arm64 build-linux-ppc64le build-linux-s390x

build-image-linux-amd64: build-linux-amd64 package-linux-amd64
build-image-linux-amd64: ARCHIVE_PATH=$(BUILD_DIR)/lifecycle-v$(LIFECYCLE_VERSION)+linux.x86-64.tgz
build-image-linux-amd64:
	$(GOCMD) run ./tools/image/main.go -daemon -lifecyclePath $(ARCHIVE_PATH) -os linux -arch amd64 -tag lifecycle:$(LIFECYCLE_IMAGE_TAG)

build-image-linux-arm64: build-linux-arm64 package-linux-arm64
build-image-linux-arm64: ARCHIVE_PATH=$(BUILD_DIR)/lifecycle-v$(LIFECYCLE_VERSION)+linux.arm64.tgz
build-image-linux-arm64:
	$(GOCMD) run ./tools/image/main.go -daemon -lifecyclePath $(ARCHIVE_PATH) -os linux -arch arm64 -tag lifecycle:$(LIFECYCLE_IMAGE_TAG)

build-image-linux-ppc64le: build-linux-ppc64le package-linux-ppc64le
build-image-linux-ppc64le: ARCHIVE_PATH=$(BUILD_DIR)/lifecycle-v$(LIFECYCLE_VERSION)+linux.ppc64le.tgz
build-image-linux-ppc64le:
	$(GOCMD) run ./tools/image/main.go -daemon -lifecyclePath $(ARCHIVE_PATH) -os linux -arch ppc64le -tag lifecycle:$(LIFECYCLE_IMAGE_TAG)

build-image-linux-s390x: build-linux-s390x package-linux-s390x
build-image-linux-s390x: ARCHIVE_PATH=$(BUILD_DIR)/lifecycle-v$(LIFECYCLE_VERSION)+linux.s390x.tgz
build-image-linux-s390x:
	$(GOCMD) run ./tools/image/main.go -daemon -lifecyclePath $(ARCHIVE_PATH) -os linux -arch s390x -tag lifecycle:$(LIFECYCLE_IMAGE_TAG)

define build_targets
build-$(1)-$(2): build-$(1)-$(2)-lifecycle build-$(1)-$(2)-symlinks build-$(1)-$(2)-launcher

build-$(1)-$(2)-lifecycle: $(BUILD_DIR)/$(1)-$(2)/lifecycle/lifecycle

$$(BUILD_DIR)/$(1)-$(2)/lifecycle/lifecycle: export GOOS:=$(1)
$$(BUILD_DIR)/$(1)-$(2)/lifecycle/lifecycle: export GOARCH:=$(2)
$$(BUILD_DIR)/$(1)-$(2)/lifecycle/lifecycle: OUT_DIR?=$$(BUILD_DIR)/$$(GOOS)-$$(GOARCH)/lifecycle
$$(BUILD_DIR)/$(1)-$(2)/lifecycle/lifecycle: $$(GOFILES)
$$(BUILD_DIR)/$(1)-$(2)/lifecycle/lifecycle:
	@echo "> Building lifecycle/lifecycle for $$(GOOS)/$$(GOARCH)..."
	mkdir -p $$(OUT_DIR)
	$$(GOENV) $$(GOBUILD) -o $$(OUT_DIR)/lifecycle -a ./cmd/lifecycle

build-$(1)-$(2)-symlinks: export GOOS:=$(1)
build-$(1)-$(2)-symlinks: export GOARCH:=$(2)
build-$(1)-$(2)-symlinks: OUT_DIR?=$$(BUILD_DIR)/$$(GOOS)-$$(GOARCH)/lifecycle
build-$(1)-$(2)-symlinks:
	@echo "> Creating phase symlinks for $$(GOOS)/$$(GOARCH)..."
	ln -sf lifecycle $$(OUT_DIR)/detector
	ln -sf lifecycle $$(OUT_DIR)/analyzer
	ln -sf lifecycle $$(OUT_DIR)/restorer
	ln -sf lifecycle $$(OUT_DIR)/builder
	ln -sf lifecycle $$(OUT_DIR)/exporter
	ln -sf lifecycle $$(OUT_DIR)/rebaser
	ln -sf lifecycle $$(OUT_DIR)/creator
	ln -sf lifecycle $$(OUT_DIR)/extender

build-$(1)-$(2)-launcher: $$(BUILD_DIR)/$(1)-$(2)/lifecycle/launcher

$$(BUILD_DIR)/$(1)-$(2)/lifecycle/launcher: export GOOS:=$(1)
$$(BUILD_DIR)/$(1)-$(2)/lifecycle/launcher: export GOARCH:=$(2)
$$(BUILD_DIR)/$(1)-$(2)/lifecycle/launcher: OUT_DIR?=$$(BUILD_DIR)/$$(GOOS)-$$(GOARCH)/lifecycle
$$(BUILD_DIR)/$(1)-$(2)/lifecycle/launcher: $$(GOFILES)
$$(BUILD_DIR)/$(1)-$(2)/lifecycle/launcher:
	@echo "> Building lifecycle/launcher for $$(GOOS)/$$(GOARCH)..."
	mkdir -p $$(OUT_DIR)
	$$(GOENV) $$(GOBUILD) -o $$(OUT_DIR)/launcher -a ./cmd/launcher
	test $$$$(du -m $$(OUT_DIR)/launcher|cut -f 1) -le 3
endef

$(foreach ga,$(GOOS_ARCHS),$(eval $(call build_targets,$(word 1, $(subst /, ,$(ga))),$(word 2, $(subst /, ,$(ga))))))

generate-sbom: run-syft-linux-amd64 run-syft-linux-arm64 run-syft-linux-ppc64le run-syft-linux-s390x

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

define install-go-tool
	@echo "> Installing $(1)..."
	$(GOCMD) install $(1)@$(shell $(GOCMD) list -m -f '{{.Version}}' $(2))
endef

install-goimports:
	@echo "> Installing goimports..."
	$(call install-go-tool,golang.org/x/tools/cmd/goimports,golang.org/x/tools)

install-yj:
	@echo "> Installing yj..."
	$(call install-go-tool,github.com/sclevine/yj,github.com/sclevine/yj)

install-mockgen:
	@echo "> Installing mockgen..."
	$(call install-go-tool,github.com/golang/mock/mockgen,github.com/golang/mock)

install-golangci-lint:
	@echo "> Installing golangci-lint..."
	$(call install-go-tool,github.com/golangci/golangci-lint/v2/cmd/golangci-lint,github.com/golangci/golangci-lint/v2)

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

package:  generate-sbom package-linux-amd64 package-linux-arm64 package-linux-ppc64le package-linux-s390x

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
