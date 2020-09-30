#!/bin/bash
#cmd to generate protobuf packages
#protoc --descriptor_set_out=sample.pb --go_out=plugins=grpc:. ./pb/sample.proto ./pb/valid.proto
protoc --descriptor_set_out=pb/sample.pb --go_out=plugins=grpc:. ./pb/sample.proto 
wire gen -output_file_prefix main- cmd/main.go


