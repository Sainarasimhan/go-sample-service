#!/bin/bash
#cmd to generate protobuf packages
protoc --go_out=plugins=grpc:. ./pb/sample.proto ./pb/valid.proto
wire gen -output_file_prefix main- cmd/main.go


