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

FROM golang:1.14-stretch

ARG GRPC_PROTOC_VER
ARG GRPC_GOGO_PROTO_TAG
ARG PLATFORM=linux-x86_64

ENV TARBALL protoc-${GRPC_PROTOC_VER}-${PLATFORM}.zip

RUN apt-get update && apt-get install unzip

RUN curl -L -o /tmp/${TARBALL} https://github.com/google/protobuf/releases/download/v${GRPC_PROTOC_VER}/${TARBALL}
RUN cd /tmp && unzip /tmp/protoc-${GRPC_PROTOC_VER}-linux-x86_64.zip -d /usr/local && rm /tmp/${TARBALL}

RUN GO111MODULE=on go get github.com/gogo/protobuf/proto@${GRPC_GOGO_PROTO_TAG} github.com/gogo/protobuf/protoc-gen-gofast@${GRPC_GOGO_PROTO_TAG}

ENV PATH $PATH:/gopath/bin

ENV PROTO_INCLUDE "/usr/local/include":"${GOPATH}/pkg/mod/github.com/gogo/protobuf@${GRPC_GOGO_PROTO_TAG}/gogoproto"
