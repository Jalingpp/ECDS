// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.2.0
// - protoc             v5.26.1
// source: storj_ac_sn.proto

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

// StorjSNACServiceClient is the client API for StorjSNACService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type StorjSNACServiceClient interface {
	StorjPutFileNotice(ctx context.Context, in *StorjClientStorageRequest, opts ...grpc.CallOption) (*StorjClientStorageResponse, error)
	StorjUpdateFileNotice(ctx context.Context, in *StorjClientUFRequest, opts ...grpc.CallOption) (*StorjClientUFResponse, error)
	StorjPreAuditSN(ctx context.Context, in *StorjPASNRequest, opts ...grpc.CallOption) (*StorjPASNResponse, error)
	StorjGetPosSN(ctx context.Context, in *StorjGAPSNRequest, opts ...grpc.CallOption) (*StorjGAPSNResponse, error)
}

type storjSNACServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewStorjSNACServiceClient(cc grpc.ClientConnInterface) StorjSNACServiceClient {
	return &storjSNACServiceClient{cc}
}

func (c *storjSNACServiceClient) StorjPutFileNotice(ctx context.Context, in *StorjClientStorageRequest, opts ...grpc.CallOption) (*StorjClientStorageResponse, error) {
	out := new(StorjClientStorageResponse)
	err := c.cc.Invoke(ctx, "/proto.StorjSNACService/StorjPutFileNotice", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *storjSNACServiceClient) StorjUpdateFileNotice(ctx context.Context, in *StorjClientUFRequest, opts ...grpc.CallOption) (*StorjClientUFResponse, error) {
	out := new(StorjClientUFResponse)
	err := c.cc.Invoke(ctx, "/proto.StorjSNACService/StorjUpdateFileNotice", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *storjSNACServiceClient) StorjPreAuditSN(ctx context.Context, in *StorjPASNRequest, opts ...grpc.CallOption) (*StorjPASNResponse, error) {
	out := new(StorjPASNResponse)
	err := c.cc.Invoke(ctx, "/proto.StorjSNACService/StorjPreAuditSN", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *storjSNACServiceClient) StorjGetPosSN(ctx context.Context, in *StorjGAPSNRequest, opts ...grpc.CallOption) (*StorjGAPSNResponse, error) {
	out := new(StorjGAPSNResponse)
	err := c.cc.Invoke(ctx, "/proto.StorjSNACService/StorjGetPosSN", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// StorjSNACServiceServer is the server API for StorjSNACService service.
// All implementations must embed UnimplementedStorjSNACServiceServer
// for forward compatibility
type StorjSNACServiceServer interface {
	StorjPutFileNotice(context.Context, *StorjClientStorageRequest) (*StorjClientStorageResponse, error)
	StorjUpdateFileNotice(context.Context, *StorjClientUFRequest) (*StorjClientUFResponse, error)
	StorjPreAuditSN(context.Context, *StorjPASNRequest) (*StorjPASNResponse, error)
	StorjGetPosSN(context.Context, *StorjGAPSNRequest) (*StorjGAPSNResponse, error)
	mustEmbedUnimplementedStorjSNACServiceServer()
}

// UnimplementedStorjSNACServiceServer must be embedded to have forward compatible implementations.
type UnimplementedStorjSNACServiceServer struct {
}

func (UnimplementedStorjSNACServiceServer) StorjPutFileNotice(context.Context, *StorjClientStorageRequest) (*StorjClientStorageResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method StorjPutFileNotice not implemented")
}
func (UnimplementedStorjSNACServiceServer) StorjUpdateFileNotice(context.Context, *StorjClientUFRequest) (*StorjClientUFResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method StorjUpdateFileNotice not implemented")
}
func (UnimplementedStorjSNACServiceServer) StorjPreAuditSN(context.Context, *StorjPASNRequest) (*StorjPASNResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method StorjPreAuditSN not implemented")
}
func (UnimplementedStorjSNACServiceServer) StorjGetPosSN(context.Context, *StorjGAPSNRequest) (*StorjGAPSNResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method StorjGetPosSN not implemented")
}
func (UnimplementedStorjSNACServiceServer) mustEmbedUnimplementedStorjSNACServiceServer() {}

// UnsafeStorjSNACServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to StorjSNACServiceServer will
// result in compilation errors.
type UnsafeStorjSNACServiceServer interface {
	mustEmbedUnimplementedStorjSNACServiceServer()
}

func RegisterStorjSNACServiceServer(s grpc.ServiceRegistrar, srv StorjSNACServiceServer) {
	s.RegisterService(&StorjSNACService_ServiceDesc, srv)
}

func _StorjSNACService_StorjPutFileNotice_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(StorjClientStorageRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(StorjSNACServiceServer).StorjPutFileNotice(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/proto.StorjSNACService/StorjPutFileNotice",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(StorjSNACServiceServer).StorjPutFileNotice(ctx, req.(*StorjClientStorageRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _StorjSNACService_StorjUpdateFileNotice_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(StorjClientUFRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(StorjSNACServiceServer).StorjUpdateFileNotice(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/proto.StorjSNACService/StorjUpdateFileNotice",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(StorjSNACServiceServer).StorjUpdateFileNotice(ctx, req.(*StorjClientUFRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _StorjSNACService_StorjPreAuditSN_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(StorjPASNRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(StorjSNACServiceServer).StorjPreAuditSN(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/proto.StorjSNACService/StorjPreAuditSN",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(StorjSNACServiceServer).StorjPreAuditSN(ctx, req.(*StorjPASNRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _StorjSNACService_StorjGetPosSN_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(StorjGAPSNRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(StorjSNACServiceServer).StorjGetPosSN(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/proto.StorjSNACService/StorjGetPosSN",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(StorjSNACServiceServer).StorjGetPosSN(ctx, req.(*StorjGAPSNRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// StorjSNACService_ServiceDesc is the grpc.ServiceDesc for StorjSNACService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var StorjSNACService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "proto.StorjSNACService",
	HandlerType: (*StorjSNACServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "StorjPutFileNotice",
			Handler:    _StorjSNACService_StorjPutFileNotice_Handler,
		},
		{
			MethodName: "StorjUpdateFileNotice",
			Handler:    _StorjSNACService_StorjUpdateFileNotice_Handler,
		},
		{
			MethodName: "StorjPreAuditSN",
			Handler:    _StorjSNACService_StorjPreAuditSN_Handler,
		},
		{
			MethodName: "StorjGetPosSN",
			Handler:    _StorjSNACService_StorjGetPosSN_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "storj_ac_sn.proto",
}
