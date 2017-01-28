#! /bin/bash

# Copyright 2016 Gravitational, Inc.
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
