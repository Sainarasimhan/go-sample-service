#!/bin/bash
#cmd to generate protobuf packages
protoc --go_out=plugins=grpc:. ./pb/sample.proto ./pb/valid.proto


