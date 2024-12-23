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

REGISTRY ?= quay.io/jparrill
IMAGE    ?= $(REGISTRY)/hypershift-oadp-plugin
VERSION  ?= main

ARCH ?= amd64
DOCKER_BUILD_ARGS ?= --platform=linux/$(ARCH)

.PHONY: local
local: build-dirs
	CGO_ENABLED=0 GOARCH=$(ARCH) go build -v -o _output/bin/$(BIN) .

.PHONY: test
test:
	CGO_ENABLED=0 go test -v -timeout 60s ./...

.PHONY: ci
ci: verify-modules local test

.PHONY: docker-build
docker-build:
	docker build -t $(IMAGE):$(VERSION) . $(DOCKER_BUILD_ARGS)

.PHONY: docker-push
docker-push:
	@docker push $(IMAGE):$(VERSION)
ifeq ($(TAG_LATEST), true)
	docker tag $(IMAGE):$(VERSION) $(IMAGE):latest
	docker push $(IMAGE):latest
endif

.PHONY: modules
modules:
	go mod tidy -compat=1.23

# verify-modules ensures Go module files are up to date
.PHONY: verify-modules
verify-modules: modules
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