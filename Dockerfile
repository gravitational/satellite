# Copyright 2017 Gravitational, Inc.
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

ARG BUILD_IMAGE
FROM ${BUILD_IMAGE} as build
ADD . /go/src/github.com/gravitational/satellite
ARG BUILD_FLAGS
RUN go build -ldflags "${BUILD_FLAGS}" -o /go/src/github.com/gravitational/satellite/build/satellite \
    github.com/gravitational/satellite/cmd/agent
RUN go build -ldflags "${BUILD_FLAGS}" -o /go/src/github.com/gravitational/satellite/build/healthz \
    github.com/gravitational/satellite/cmd/healthz

FROM quay.io/gravitational/debian-tall:buster
MAINTAINER Grvitational Inc <admin@gravitational.com>

COPY --from=build /go/src/github.com/gravitational/satellite/build/satellite /usr/local/bin/
COPY --from=build /go/src/github.com/gravitational/satellite/build/healthz /usr/local/bin/

EXPOSE 7575 8080
