//+build mage

/*
Copyright 2019 Gravitational, Inc.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gravitational/trace"
	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

var (
	// buildContainer is a docker container used to build go binaries
	buildContainer = "golang:1.14.3"

	// golangciVersion is the version of golangci-lint to use for linting
	// https://github.com/golangci/golangci-lint/releases
	golangciVersion = "v1.27.0"

	// nethealthRegistryImage is the docker tag to use to push the nethealth container to the requested registry
	nethealthRegistryImage = env("NETHEALTH_REGISTRY_IMAGE", "quay.io/gravitational/nethealth-dev")

	// baseImage is the base OS image to use for containers
	baseImage = "gcr.io/distroless/base-debian10"

	// buildVersion allows override of the version string from env variable
	buildVersion = env("BUILD_VERSION", "")

	grpcProtocVersion = env("GRPC_PROTOC_VER", "3.14.0")     // version of protoc
	grpcGogoProtoTag  = env("GRPC_GOGO_PROTO_TAG", "v1.3.1") // gogo version tag
	grpcGatewayTag    = env("GRPC_GATEWAY_TAG", "v1.16.0")   // grpc gateway version tag
)

type Build mg.Namespace

// All builds all binaries
func (Build) All() error {
	mg.Deps(Build.Nethealth, Build.Satellite, Build.Healthz)

	return nil
}

// BuildContainer creates a docker container as a consistent golang environment to use for software builds and tests
func (Build) BuildContainer() error {
	fmt.Println("\n=====> Creating build container...\n")
	return trace.Wrap(sh.RunV(
		"docker", "build", "--pull",
		"--tag", fmt.Sprint("satellite-build:", version()),
		"--build-arg", fmt.Sprint("BUILD_IMAGE=", buildContainer),
		"--build-arg", fmt.Sprint("GOLANGCI_VER=", golangciVersion),
		"-f", "Dockerfile.build",
		".",
	))
}

// Nethealth builds the nethealth binary
func (Build) Nethealth() error {
	fmt.Println("\n=====> Building nethealth binary...\n")
	return trace.Wrap(sh.RunV(
		"go", "build",
		"-ldflags", flags(),
		"-o", outPath("nethealth"),
		"github.com/gravitational/satellite/cmd/nethealth",
	))
}

// Satellite builds the satellite binary
func (Build) Satellite() error {
	fmt.Println("\n=====> Building satellite binary...\n")
	mg.Deps(Codegen.Grpc)
	return trace.Wrap(sh.RunV(
		"go", "build",
		"-o", outPath("satellite"),
		"-ldflags", flags(),
		"github.com/gravitational/satellite/cmd/agent",
	))
}

// Healthz builds the healthz binary
func (Build) Healthz() error {
	fmt.Println("\n=====> Building healthz binary...\n")
	return trace.Wrap(sh.RunV(
		"go", "build",
		"-o", outPath("healthz"),
		"-ldflags", flags(),
		"github.com/gravitational/satellite/cmd/healthz",
	))
}

type Publish mg.Namespace

// Nethealth tags and publishes the nethealth docker container to the configured registry
func (Publish) Nethealth() error {
	mg.Deps(Docker.Nethealth)
	fmt.Println("\n=====> Publishing nethealth docker image...\n")

	err := sh.RunV(
		"docker", "tag",
		fmt.Sprint("nethealth:", version()),
		fmt.Sprint(nethealthRegistryImage, ":", version()),
	)
	if err != nil {
		return trace.Wrap(err)
	}

	return trace.Wrap(sh.RunV(
		"docker", "push", fmt.Sprint(nethealthRegistryImage, ":", version()),
	))
}

type Docker mg.Namespace

// Nethealth builds nethealth docker image
func (Docker) Nethealth() error {
	mg.Deps(Build.BuildContainer)
	fmt.Println("\n=====> Building nethealth docker image...\n")

	// Force pull upstream images to refresh any local caches
	err := sh.RunV("docker", "pull", baseImage)
	if err != nil {
		return trace.Wrap(err)
	}

	return trace.Wrap(sh.RunV(
		"docker", "build",
		"--tag", fmt.Sprint("nethealth:", version()),
		"--build-arg", fmt.Sprint("BUILD_VERSION=", version()),
		"--build-arg", fmt.Sprint("BASE_IMAGE=", baseImage),
		"--build-arg", fmt.Sprint("BUILD_IMAGE=satellite-build:", version()),
		"-f", "Dockerfile.nethealth",
		".",
	))
}

// Healthz builds the healthz container
func (Docker) Healthz() error {
	mg.Deps(Build.BuildContainer)
	fmt.Println("\n=====> Building Satellite Docker Image...\n")
	return trace.Wrap(sh.RunV(
		"docker", "build",
		"--tag", fmt.Sprint("satellite:", version()),
		"--build-arg", fmt.Sprint("BUILD_IMAGE=satellite-build:", version()),
		"--build-arg", fmt.Sprintf("BUILD_FLAGS=%v", flags()),
		".",
	))
}

type Test mg.Namespace

// All runs all test targets
func (Test) All() error {
	mg.Deps(Test.Unit, Test.Lint, Test.Style)
	return nil
}

// Unit runs unit tests with the race detector enabled
func (Test) Unit() error {
	mg.Deps(Build.BuildContainer)
	fmt.Println("\n=====> Running Satellite Unit Tests...\n")
	return trace.Wrap(sh.RunV(
		"docker", "run", "--rm",
		fmt.Sprintf("--volume=%v:/go/src/github.com/gravitational/satellite", srcDir()),
		`--env="GOCACHE=/go/src/github.com/gravitational/satellite/build/cache/go"`,
		`-w=/go/src/github.com/gravitational/satellite/`,
		fmt.Sprint("satellite-build:", version()),
		"go", "--", "test", "-mod", "vendor", "./...", "-race",
	))
}

// Lint runs lints against the repo (golangci)
func (Test) Lint() error {
	mg.Deps(Build.BuildContainer)
	fmt.Println("\n=====> Linting Satellite...\n")
	return trace.Wrap(sh.RunV(
		"docker", "run", "--rm",
		fmt.Sprintf("--volume=%v:/go/src/github.com/gravitational/satellite", srcDir()),
		`--env="GOCACHE=/go/src/github.com/gravitational/satellite/build/cache/go"`,
		fmt.Sprint("satellite-build:", version()),
		"golangci-lint", "run", "--new", "--deadline=30m", "--enable-all",
		"--disable=gochecknoglobals", "--disable=gochecknoinits", "--disable=gomodguard",
	))
}

// Style validates that licenses exist on each source file
func (Test) Style() error {
	fmt.Println("\n=====> Running scripts/validate-license.sh...\n")
	return trace.Wrap(sh.RunV("scripts/validate-license.sh"))
}

type Codegen mg.Namespace

// Buildbox creates a docker container for gRPC code generation
func (Codegen) Buildbox() error {
	fmt.Println("\n=====> Building GRPC Buildbox...\n")
	return trace.Wrap(sh.RunV(
		"docker", "build",
		"--tag", fmt.Sprint("satellite-grpc-buildbox:", version()),
		"--build-arg", fmt.Sprint("GRPC_PROTOC_VER=", grpcProtocVersion),
		"--build-arg", fmt.Sprint("GRPC_GOGO_PROTO_TAG=", grpcGogoProtoTag),
		"--build-arg", fmt.Sprint("GRPC_GATEWAY_TAG=", grpcGatewayTag),
		"-f", "build.assets/grpc/Dockerfile",
		".",
	))
}

// Grpc runs the gRPC code generator
func (Codegen) Grpc() error {
	mg.Deps(Codegen.Buildbox)
	fmt.Println("\n=====> Running GRPC Codegen inside docker...\n")
	return trace.Wrap(sh.RunV(
		"docker", "run",
		fmt.Sprintf("--volume=%v:/go/src/github.com/gravitational/satellite:delegated", srcDir()),
		fmt.Sprint("satellite-grpc-buildbox:", version()),
		"sh", "-c",
		"cd /go/src/github.com/gravitational/satellite/ && go run mage.go internal:grpcAgent internal:grpcDebug",
	))
}

type Internal mg.Namespace

// GrpcAgent (Internal) generates protobuf stubs for Agent server
// The task is called from codeGen:grpc target inside docker
func (Internal) GrpcAgent() error {
	fmt.Println("\n=====> Running protoc...\n")
	err := os.Chdir(filepath.Join(srcDir(), "agent/proto/agentpb"))
	if err != nil {
		return trace.ConvertSystemError(err)
	}
	protoFiles, err := filepath.Glob("*.proto")
	if err != nil {
		return trace.Wrap(err)
	}
	args := []string{fmt.Sprint("-I=.:", os.Getenv("PROTO_INCLUDE")), "--gofast_out=plugins=grpc:."}
	return trace.Wrap(sh.RunV(
		"protoc",
		append(args, protoFiles...)...,
	))
}

// GrpcDebug (Internal) generates protobuf stubs for Debug server
func (Internal) GrpcDebug() error {
	fmt.Println("\n=====> Running protoc...\n")
	err := os.Chdir(filepath.Join(srcDir(), "agent/proto"))
	if err != nil {
		return trace.ConvertSystemError(err)
	}
	protoFiles, err := filepath.Glob("debug/*.proto")
	if err != nil {
		return trace.Wrap(err)
	}
	args := []string{fmt.Sprint("-I=.:", os.Getenv("PROTO_INCLUDE")), "--gofast_out=plugins=grpc:."}
	return trace.Wrap(sh.RunV(
		"protoc",
		append(args, protoFiles...)...,
	))
}

// Clean removes all build artifacts
func Clean() error {
	buildDir := filepath.Join(srcDir(), "build")
	fmt.Println("\n=====> Cleaning: ", buildDir)
	return trace.ConvertSystemError(os.RemoveAll(buildDir))
}

func srcDir() string {
	return filepath.Join(os.Getenv("GOPATH"), "src/github.com/gravitational/satellite/")
}

func flags() string {
	timestamp := time.Now().Format(time.RFC3339)
	hash := hash()
	version := version()

	flags := []string{
		fmt.Sprint(`-X main.timestamp=`, timestamp),
		fmt.Sprint(`-X main.commitHash=`, hash),
		fmt.Sprint(`-X main.gitTag=`, version),
		"-s -w", // shrink the binary
	}

	return strings.Join(flags, " ")
}

// hash returns the git hash for the current repository or "" if none.
func hash() string {
	hash, _ := sh.Output("git", "rev-parse", "--short", "HEAD")
	return hash
}

// version returns the git tag for the current branch or "" if none.
func version() string {
	if buildVersion != "" {
		return buildVersion
	}
	longTag, _ := sh.Output("git", "describe", "--tags", "--dirty")

	return longTag
}

// env, loads a variable from the environment, or uses the provided default
func env(env, d string) string {
	if os.Getenv(env) != "" {
		return os.Getenv(env)
	}
	return d
}

func outPath(binaryName string) string {
	return filepath.Join(srcDir(), "build", binaryName)
}
