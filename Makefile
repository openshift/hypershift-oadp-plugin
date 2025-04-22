# Copyright 2024 Red Hat Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

PKG := github.com/openshift/hypershift-oadp-plugin
BIN := hypershift-oadp-plugin
IMG ?= quay.io/hypershift/hypershift-oadp-plugin:latest
VERSION ?= $(shell git describe --tags --always)

# Supported architectures and platforms
ARCHS ?= amd64 arm64
DOCKER_BUILD_ARGS ?= --platform=linux/$(ARCH)
GO=GO111MODULE=on GOWORK=off GOFLAGS=-mod=vendor go

.PHONY: install-goreleaser
install-goreleaser:
 	## Latest version of goreleaser v1. V2 requires go 1.24+
	@echo "Installing goreleaser..."
	@GOFLAGS= go install github.com/goreleaser/goreleaser@v1.26.2
	@echo "Goreleaser installed successfully!"

.PHONY: local
local: verify install-goreleaser build-dirs
	goreleaser build --snapshot --clean
	@mkdir -p dist/$(BIN)_$(VERSION)
	@mv dist/$(BIN)_*/* dist/$(BIN)_$(VERSION)/
	@rm -rf dist/$(BIN)_darwin_* dist/$(BIN)_linux_*

.PHONY: release
release: verify install-goreleaser
	goreleaser release --clean

.PHONY: release-local
release-local: verify install-goreleaser build-dirs
	GORELEASER_CURRENT_TAG=$(VERSION) goreleaser build --clean

.PHONY: tests
test:
	$(GO) test -v -timeout 60s ./...

.PHONY: cover
cover:
	$(GO) test --cover -timeout 60s ./...

.PHONY: deps
deps:
	$(GO) mod tidy && $(GO) mod vendor

.PHONY: verify
verify: verify-modules test

.PHONY: docker-build
docker-build:
	docker build -t ${IMG} .

.PHONY: docker-push
docker-push:
	docker push ${IMG}

# verify-modules ensures Go module files are up to date
.PHONY: verify-modules
verify-modules: deps
	@if !(git diff --quiet HEAD -- go.sum go.mod); then \
		echo "go module files are out of date, please commit the changes to go.mod and go.sum"; exit 1; \
	fi

.PHONY: build-dirs
build-dirs:
	@mkdir -p dist

# clean removes build artifacts from the local environment.
.PHONY: clean
clean:
	@echo "cleaning"
	rm -rf _output dist