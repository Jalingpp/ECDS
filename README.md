# ECDS

This is an Erasure Coded Decentralized Storage System. ECDS implements functions such as encoded file writing, complete reading, single data block updating, and asynchronous storage auditing. 

The installation and testing steps for ECDS are as follows.

## 1 Install gRPC 

The golang-grpc package provides a library about gRPC. The installation command is: `go get -u google.golang.org/grpc`

Install two packages to support the processing of protobuf files:

```
go get -u github.com/golang/protobuf
go get -u github.com/golang/protobuf/protoc-gen-go
```

## 2 Install Protocol Buffers

Protocol Buffers is a language-neutral, platform-neutral, extensible mechanism for serializing structured data and is used as a data exchange format. gRPC uses protoc as the protocol processing tool.

Step1. Download the package corresponding to the current system: `protoc-26.1-linux-x86_64.zip`，download URL: https://github.com/protocolbuffers/protobuf/releases

Step2. Extract and set the system PATH. Copy `bin/ptotoc.exe` to `/root/pkg/protoc/bin`，configure the system PATH using the commond `export PATH=/root/pkg/protoc/bin:$PATH`.

Step3. Test proto compilation. Create a file named `test.proto` in a directory of your project and write the following code. Execute the command `protoc --go_out=. --go-grpc_out=. test.proto` to generate two files `test_grpc.pb.go` and `test.pb.go` in the same directory.

```go
syntax = "proto3";

option go_package = "./";

service MyService {
    rpc Process(Request) returns (Response);
}

message Request {
    string message = 1;
}

message Response {
    string result = 1;
}
```

NOTICE：①`option go_package = "./";` means that the output directory is the directory where `test.proto` is located,If it is `option go_package = "./pb";`, then a `pb` directory will be created in the directory where `test.proto`is located; ② If you encounter the error `plugins are not supported`, you can try resolving it by downloading the latest version. The command is `go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest`; ③You can use a `.sh` script to compile `.proto` files. For example, create `build.sh` and input the following code. Then, use command `chmod +x build.sh` to add execute permissions and using command `./build.sh` to execute the script.

```bash
#!/bin/bash

# Execute the protoc command to generate Go and gRPC code.
protoc --go_out=. --go-grpc_out=. test.proto
```

The result after executing the script is as shown in the following image:

<img src="image.png" alt="image" style="float:left; margin-right:10px;" />

## 3 Install PBC

