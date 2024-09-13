#!/bin/bash

# 执行 protoc 命令生成 Go 代码和 gRPC 相关的代码
/root/pkg/protoc/bin/protoc --go_out=. --go-grpc_out=. client_ac.proto
/root/pkg/protoc/bin/protoc --go_out=. --go-grpc_out=. client_sn.proto
/root/pkg/protoc/bin/protoc --go_out=. --go-grpc_out=. ac_sn.proto
/root/pkg/protoc/bin/protoc --go_out=. --go-grpc_out=. storj_client_ac.proto
/root/pkg/protoc/bin/protoc --go_out=. --go-grpc_out=. storj_ac_sn.proto
/root/pkg/protoc/bin/protoc --go_out=. --go-grpc_out=. storj_client_sn.proto
/root/pkg/protoc/bin/protoc --go_out=. --go-grpc_out=. filecoin_client_ac.proto
/root/pkg/protoc/bin/protoc --go_out=. --go-grpc_out=. filecoin_client_sn.proto
/root/pkg/protoc/bin/protoc --go_out=. --go-grpc_out=. filecoin_ac_sn.proto
/root/pkg/protoc/bin/protoc --go_out=. --go-grpc_out=. sia_client_ac.proto
/root/pkg/protoc/bin/protoc --go_out=. --go-grpc_out=. sia_client_sn.proto
/root/pkg/protoc/bin/protoc --go_out=. --go-grpc_out=. sia_ac_sn.proto