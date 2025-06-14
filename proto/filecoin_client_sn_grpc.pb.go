// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.2.0
// - protoc             v5.26.1
// source: filecoin_client_sn.proto

package __

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// FilecoinSNServiceClient is the client API for FilecoinSNService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type FilecoinSNServiceClient interface {
	FilecoinPutFile(ctx context.Context, in *FilecoinPutFRequest, opts ...grpc.CallOption) (*FilecoinPutFResponse, error)
	FilecoinGetFile(ctx context.Context, in *FilecoinGetFRequest, opts ...grpc.CallOption) (*FilecoinGetFResponse, error)
	FilecoinUpdateFile(ctx context.Context, in *FilecoinUpdFRequest, opts ...grpc.CallOption) (*FilecoinUpdFResponse, error)
	FilecoinGetSNStorageCost(ctx context.Context, in *FilecoinGSNSCRequest, opts ...grpc.CallOption) (*FilecoinGSNSCResponse, error)
}

type filecoinSNServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewFilecoinSNServiceClient(cc grpc.ClientConnInterface) FilecoinSNServiceClient {
	return &filecoinSNServiceClient{cc}
}

func (c *filecoinSNServiceClient) FilecoinPutFile(ctx context.Context, in *FilecoinPutFRequest, opts ...grpc.CallOption) (*FilecoinPutFResponse, error) {
	out := new(FilecoinPutFResponse)
	err := c.cc.Invoke(ctx, "/proto.FilecoinSNService/FilecoinPutFile", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *filecoinSNServiceClient) FilecoinGetFile(ctx context.Context, in *FilecoinGetFRequest, opts ...grpc.CallOption) (*FilecoinGetFResponse, error) {
	out := new(FilecoinGetFResponse)
	err := c.cc.Invoke(ctx, "/proto.FilecoinSNService/FilecoinGetFile", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *filecoinSNServiceClient) FilecoinUpdateFile(ctx context.Context, in *FilecoinUpdFRequest, opts ...grpc.CallOption) (*FilecoinUpdFResponse, error) {
	out := new(FilecoinUpdFResponse)
	err := c.cc.Invoke(ctx, "/proto.FilecoinSNService/FilecoinUpdateFile", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *filecoinSNServiceClient) FilecoinGetSNStorageCost(ctx context.Context, in *FilecoinGSNSCRequest, opts ...grpc.CallOption) (*FilecoinGSNSCResponse, error) {
	out := new(FilecoinGSNSCResponse)
	err := c.cc.Invoke(ctx, "/proto.FilecoinSNService/FilecoinGetSNStorageCost", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// FilecoinSNServiceServer is the server API for FilecoinSNService service.
// All implementations must embed UnimplementedFilecoinSNServiceServer
// for forward compatibility
type FilecoinSNServiceServer interface {
	FilecoinPutFile(context.Context, *FilecoinPutFRequest) (*FilecoinPutFResponse, error)
	FilecoinGetFile(context.Context, *FilecoinGetFRequest) (*FilecoinGetFResponse, error)
	FilecoinUpdateFile(context.Context, *FilecoinUpdFRequest) (*FilecoinUpdFResponse, error)
	FilecoinGetSNStorageCost(context.Context, *FilecoinGSNSCRequest) (*FilecoinGSNSCResponse, error)
	mustEmbedUnimplementedFilecoinSNServiceServer()
}

// UnimplementedFilecoinSNServiceServer must be embedded to have forward compatible implementations.
type UnimplementedFilecoinSNServiceServer struct {
}

func (UnimplementedFilecoinSNServiceServer) FilecoinPutFile(context.Context, *FilecoinPutFRequest) (*FilecoinPutFResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method FilecoinPutFile not implemented")
}
func (UnimplementedFilecoinSNServiceServer) FilecoinGetFile(context.Context, *FilecoinGetFRequest) (*FilecoinGetFResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method FilecoinGetFile not implemented")
}
func (UnimplementedFilecoinSNServiceServer) FilecoinUpdateFile(context.Context, *FilecoinUpdFRequest) (*FilecoinUpdFResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method FilecoinUpdateFile not implemented")
}
func (UnimplementedFilecoinSNServiceServer) FilecoinGetSNStorageCost(context.Context, *FilecoinGSNSCRequest) (*FilecoinGSNSCResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method FilecoinGetSNStorageCost not implemented")
}
func (UnimplementedFilecoinSNServiceServer) mustEmbedUnimplementedFilecoinSNServiceServer() {}

// UnsafeFilecoinSNServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to FilecoinSNServiceServer will
// result in compilation errors.
type UnsafeFilecoinSNServiceServer interface {
	mustEmbedUnimplementedFilecoinSNServiceServer()
}

func RegisterFilecoinSNServiceServer(s grpc.ServiceRegistrar, srv FilecoinSNServiceServer) {
	s.RegisterService(&FilecoinSNService_ServiceDesc, srv)
}

func _FilecoinSNService_FilecoinPutFile_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(FilecoinPutFRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FilecoinSNServiceServer).FilecoinPutFile(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/proto.FilecoinSNService/FilecoinPutFile",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FilecoinSNServiceServer).FilecoinPutFile(ctx, req.(*FilecoinPutFRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _FilecoinSNService_FilecoinGetFile_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(FilecoinGetFRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FilecoinSNServiceServer).FilecoinGetFile(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/proto.FilecoinSNService/FilecoinGetFile",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FilecoinSNServiceServer).FilecoinGetFile(ctx, req.(*FilecoinGetFRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _FilecoinSNService_FilecoinUpdateFile_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(FilecoinUpdFRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FilecoinSNServiceServer).FilecoinUpdateFile(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/proto.FilecoinSNService/FilecoinUpdateFile",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FilecoinSNServiceServer).FilecoinUpdateFile(ctx, req.(*FilecoinUpdFRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _FilecoinSNService_FilecoinGetSNStorageCost_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(FilecoinGSNSCRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FilecoinSNServiceServer).FilecoinGetSNStorageCost(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/proto.FilecoinSNService/FilecoinGetSNStorageCost",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FilecoinSNServiceServer).FilecoinGetSNStorageCost(ctx, req.(*FilecoinGSNSCRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// FilecoinSNService_ServiceDesc is the grpc.ServiceDesc for FilecoinSNService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var FilecoinSNService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "proto.FilecoinSNService",
	HandlerType: (*FilecoinSNServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "FilecoinPutFile",
			Handler:    _FilecoinSNService_FilecoinPutFile_Handler,
		},
		{
			MethodName: "FilecoinGetFile",
			Handler:    _FilecoinSNService_FilecoinGetFile_Handler,
		},
		{
			MethodName: "FilecoinUpdateFile",
			Handler:    _FilecoinSNService_FilecoinUpdateFile_Handler,
		},
		{
			MethodName: "FilecoinGetSNStorageCost",
			Handler:    _FilecoinSNService_FilecoinGetSNStorageCost_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "filecoin_client_sn.proto",
}
