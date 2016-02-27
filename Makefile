.PHONY: sloccount clean all flags test test-package test-grep-package cover-package

REPODIR = $(shell pwd)
BUILDDIR = $(REPODIR)/build
OUT = $(BUILDDIR)/satellite
PWD := $(shell pwd)
export

.DEFAULT_GOAL = all

all: $(OUT)

$(OUT): flags
	go install github.com/gravitational/satellite/vendor/github.com/mattn/go-sqlite3
	go build -o $@ -ldflags $(LINKFLAGS) github.com/gravitational/satellite/cmd/agent

clean:
	@rm -rf $(OUTDIR)

flags:
	$(eval LINKFLAGS := "$(shell linkflags -pkg=$(PWD) -verpkg=github.com/gravitational/satellite/vendor/github.com/gravitational/version)")

sloccount:
	find . -path ./vendor -prune -o -name "*.go" -print0 | xargs -0 wc -l

test:
	go test -v -test.parallel=0 ./agent/...

test-package: $(OUT)
	go test -v -test.parallel=0 ./$(p)

test-grep-package: $(OUT)
	go test -v ./$(p) -check.f=$(e)

cover-package:
	go test -v ./$(p)  -coverprofile=$(BUILDDIR)/coverage.out
	go tool cover -html=$(BUILDDIR)/coverage.out




