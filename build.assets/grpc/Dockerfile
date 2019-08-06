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

FROM golang:1.10-stretch

ARG GRPC_PROTOC_VER
ARG GRPC_GOGO_PROTO_TAG
ARG GRPC_GATEWAY_TAG
ARG PLATFORM=linux-x86_64

ENV TARBALL protoc-${GRPC_PROTOC_VER}-${PLATFORM}.zip
ENV GOGOPROTO_ROOT ${GOPATH}/src/github.com/gogo/protobuf

RUN apt-get update && apt-get install unzip

RUN curl -L -o /tmp/${TARBALL} https://github.com/google/protobuf/releases/download/v${GRPC_PROTOC_VER}/${TARBALL}
RUN cd /tmp && unzip /tmp/protoc-${GRPC_PROTOC_VER}-linux-x86_64.zip -d /usr/local && rm /tmp/${TARBALL}

RUN go get -u github.com/gogo/protobuf/proto github.com/gogo/protobuf/protoc-gen-gogo github.com/gogo/protobuf/gogoproto golang.org/x/tools/cmd/goimports github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway github.com/grpc-ecosystem/grpc-gateway/protoc-gen-swagger
RUN cd ${GOPATH}/src/github.com/gogo/protobuf && git reset --hard ${GRPC_GOGO_PROTO_TAG} && make install
RUN cd ${GOPATH}/src/github.com/grpc-ecosystem/grpc-gateway && git reset --hard ${GRPC_GATEWAY_TAG} && go install ./protoc-gen-grpc-gateway

ENV PATH $PATH:/gopath/bin

ENV PROTO_INCLUDE "/usr/local/include":"${GOPATH}/src":"${GOPATH}/src/github.com/gogo/protobuf/protobuf":"${GOPATH}/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis":"${GOPATH}/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis":"${GOGOPROTO_ROOT}:${GOGOPROTO_ROOT}/protobuf"

