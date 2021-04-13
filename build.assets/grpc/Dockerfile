# Copyright 2019 Gravitational, Inc.
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

# Documentation on getting started with protobuf:
# https://github.com/gogo/protobuf#getting-started

# Requires 1.9+
FROM golang:1.16-buster

ARG GRPC_PROTOC_VER
ARG PLATFORM=linux-x86_64
ARG TARBALL=protoc-${GRPC_PROTOC_VER}-${PLATFORM}.zip
ARG PROTOC_PATH=/usr/local
ARG PROTOC_INCLUDE=${PROTOC_PATH}/include

ARG GRPC_GOGO_PROTO_TAG
ARG GOGO_PROTO_INCLUDE=${GOPATH}/pkg/mod/github.com/gogo/protobuf@${GRPC_GOGO_PROTO_TAG}/gogoproto

ENV PROTO_INCLUDE "${PROTOC_INCLUDE}":"${GOGO_PROTO_INCLUDE}"

RUN apt-get update && apt-get install unzip

# Install and extract standard protocol buffer implementation from
# https://github.com/protocolbuffers/protobuf
# Standard .proto files will be extracted under ${PROTOC_PATH}/include.
RUN curl -L -o /tmp/${TARBALL} https://github.com/protocolbuffers/protobuf/releases/download/v${GRPC_PROTOC_VER}/${TARBALL}
RUN cd /tmp && unzip /tmp/${TARBALL} -d ${PROTOC_PATH} && rm /tmp/${TARBALL}

# These binaries enable use of gogoprotobuf extensions
# https://github.com/gogo/protobuf/blob/master/extensions.md
RUN go get github.com/gogo/protobuf/proto@${GRPC_GOGO_PROTO_TAG}
RUN go install github.com/gogo/protobuf/protoc-gen-gofast@${GRPC_GOGO_PROTO_TAG}
