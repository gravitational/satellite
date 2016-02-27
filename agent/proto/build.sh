#! /bin/bash
protoc_home=~/opt/google/protobuf/bin
$protoc_home/protoc --go_out=plugins=grpc:. agentpb/agent.proto
