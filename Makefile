.PHONY: binaries sloccount clean all flags test test-style test-package test-grep-package cover-package

REPODIR = $(shell pwd)
BUILDDIR = $(REPODIR)/build
PWD := $(shell pwd)
export

.DEFAULT_GOAL = all

all: binaries

binaries: $(BUILDDIR)/satellite $(BUILDDIR)/healthz

$(BUILDDIR)/satellite: linkflags
	go build -o $@ -ldflags $(LINKFLAGS) github.com/gravitational/satellite/cmd/agent

$(BUILDDIR)/healthz: linkflags
	go build -o $@ -ldflags $(LINKFLAGS) github.com/gravitational/satellite/cmd/healthz

docker-image: binaries dockerflags
	docker build -t satellite:$(DOCKERFLAGS) $(PWD)

clean:
	@rm -rf $(OUTDIR)

dockerflags:
	$(eval DOCKERFLAGS := "$(shell linkflags -docker-tag -pkg=$(PWD) -verpkg=github.com/gravitational/satellite/vendor/github.com/gravitational/version)")

linkflags:
	$(eval LINKFLAGS := "$(shell linkflags -pkg=$(PWD) -verpkg=github.com/gravitational/satellite/vendor/github.com/gravitational/version)")

sloccount:
	find . -path ./vendor -prune -o -name "*.go" -print0 | xargs -0 wc -l

test:
	go test -test.parallel=0 -race ./agent/...
	go test -test.parallel=0 -race ./monitoring/...

test-style:
	@scripts/validate-license.sh

test-package: binaries
	go test -v -test.parallel=0 ./$(p)

test-grep-package: binaries
	go test -v ./$(p) -check.f=$(e)

cover-package:
	go test -v ./$(p)  -coverprofile=$(BUILDDIR)/coverage.out
	go tool cover -html=$(BUILDDIR)/coverage.out
