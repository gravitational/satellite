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

test-package:
	go test -v -test.parallel=0 ./$(p)

test-grep-package:
	go test -v ./$(p) -check.f=$(e)

cover-package:
	go test -v ./$(p)  -coverprofile=$(BUILDDIR)/coverage.out
	go tool cover -html=$(BUILDDIR)/coverage.out


PROTOC_VER ?= 3.0.0
GOGO_PROTO_TAG ?= v0.3
GRPC_GATEWAY_TAG ?= v1.1.0
PLATFORM := linux-x86_64
GRPC_API := agent/proto/agentpb
BUILDBOX_TAG := satellite-grpc-buildbox:0.0.1

# buildbox builds docker buildbox image used to compile binaries and generate GRPc stuff
.PHONY: buildbox
buildbox:
	cd build.assets/grpc && docker build \
          --build-arg PROTOC_VER=$(PROTOC_VER) \
          --build-arg GOGO_PROTO_TAG=$(GOGO_PROTO_TAG) \
          --build-arg GRPC_GATEWAY_TAG=$(GRPC_GATEWAY_TAG) \
          --build-arg PLATFORM=$(PLATFORM) \
          -t $(BUILDBOX_TAG) .

# grpc generates Go source code from protobuf specifications
# using docker buildbox image
.PHONY: grpc
grpc: buildbox
	docker run -v $(shell pwd):/go/src/github.com/gravitational/satellite $(BUILDBOX_TAG) make -C /go/src/github.com/gravitational/satellite buildbox-grpc

# buildbox-gpc generates Go source code from protobuf specifications
.PHONY: buildbox-grpc
buildbox-grpc:
# standard GRPC output
	echo $$PROTO_INCLUDE
	cd $(GRPC_API) && protoc -I=.:$$PROTO_INCLUDE \
      --gofast_out=plugins=grpc:.\
    *.proto
