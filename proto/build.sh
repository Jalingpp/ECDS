#!/bin/bash

# 执行 protoc 命令生成 Go 代码和 gRPC 相关的代码
# protoc --go_out=. --go-grpc_out=. client_ac.proto
protoc --go_out=. --go-grpc_out=. client_sn.proto