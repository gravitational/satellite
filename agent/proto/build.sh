#! /bin/bash
set -e

if ! [[ "$0" =~ "./build.sh" ]]; then
	echo "must be run from agent/proto directory"
	exit 255
fi

if ! [[ $(protoc --version) =~ "3.0.0" ]]; then
	echo "could not find protoc 3.0.0 in PATH"
	echo "Install from https://github.com/google/protobuf before running this script"
	exit 255
fi

protoc --go_out=plugins=grpc:. agentpb/agent.proto
