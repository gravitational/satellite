#!/usr/bin/env make -f

LINKFLAGS_BIN ?= $(shell which linkflags)
PWD := $(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))
APPLICATION ?= $(shell basename $(PWD))
NOROOT := -u $$(id -u):$$(id -g)
SRCDIR := /go/src/github.com/gravitational/$(APPLICATION)
DOCKERFLAGS := --rm=true $(NOROOT) -v $(LINKFLAGS_BIN):/usr/bin/linkflags:ro -v $(PWD):$(SRCDIR) -w $(SRCDIR)
BUILDIMAGE := quay.io/gravitational/debian-venti:go1.7-jessie
BUILDDIR = $(SRCDIR)/build
export

.DEFAULT_GOAL = all

.PHONY: all
all: build

.PHONY: binaries
binaries: $(BUILDDIR)/satellite $(BUILDDIR)/healthz

.PHONY: build
build:
	docker run $(DOCKERFLAGS) $(BUILDIMAGE) make clean binaries

$(BUILDDIR)/satellite: linkflags
	go build -o $@ -ldflags $(LINKFLAGS) github.com/gravitational/satellite/cmd/agent

$(BUILDDIR)/healthz: linkflags
	go build -o $@ -ldflags $(LINKFLAGS) github.com/gravitational/satellite/cmd/healthz

.PHONY: docker-image
docker-image: build dockerflags
	docker build -t satellite:$(DOCKERFLAGS) $(PWD)

.PHONY: clean
clean:
	@rm -rf $(OUTDIR)

.PHONY: dockerflags
dockerflags:
	$(eval DOCKERFLAGS := "$(shell linkflags -docker-tag -pkg=$(PWD) -verpkg=github.com/gravitational/satellite/vendor/github.com/gravitational/version)")

.PHONY: linkflags
linkflags:
	$(eval LINKFLAGS := "$(shell linkflags -pkg=$(PWD) -verpkg=github.com/gravitational/satellite/vendor/github.com/gravitational/version)")

.PHONY: sloccount
sloccount:
	find . -path ./vendor -prune -o -name "*.go" -print0 | xargs -0 wc -l

.PHONY: test
test:
	go test -test.parallel=0 -race ./agent/...
	go test -test.parallel=0 -race ./monitoring/...

.PHONY: test-style
test-style:
	@scripts/validate-license.sh

.PHONY: test-package
test-package: binaries
	go test -v -test.parallel=0 ./$(p)

.PHONY: test-grep-package
test-grep-package: binaries
	go test -v ./$(p) -check.f=$(e)

.PHONY: cover-package
cover-package:
	docker run $(DOCKERFLAGS) $(BUILDIMAGE) make cover-package-in-docker

.PHONY: cover-package-in-docker
cover-package-in-docker:
	go test -v ./$(p)  -coverprofile=$(BUILDDIR)/coverage.out
	go tool cover -html=$(BUILDDIR)/coverage.out
