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

ARCH ?= amd64
DOCKER_BUILD_ARGS ?= --platform=linux/$(ARCH)
GO=GO111MODULE=on GOWORK=off GOFLAGS=-mod=vendor go


.PHONY: local
local: build-dirs
	$(GO) build -v -o _output/bin/$(BIN) .

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
verify: verify-modules local test

.PHONY: docker-build
docker-build:
	docker build -t ${IMG} . $(DOCKER_BUILD_ARGS)

.PHONY: docker-push
docker-push:
	@docker push ${IMG}

# verify-modules ensures Go module files are up to date
.PHONY: verify-modules
verify-modules: deps
	@if !(git diff --quiet HEAD -- go.sum go.mod); then \
		echo "go module files are out of date, please commit the changes to go.mod and go.sum"; exit 1; \
	fi

.PHONY: build-dirs
build-dirs:
	@mkdir -p _output/bin/$(ARCH)

# clean removes build artifacts from the local environment.
.PHONY: clean
clean:
	@echo "cleaning"
	rm -rf _output