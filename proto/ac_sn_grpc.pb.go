// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.2.0
// - protoc             v5.26.1
// source: ac_sn.proto

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

// SNACServiceClient is the client API for SNACService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type SNACServiceClient interface {
	PutDataShardNotice(ctx context.Context, in *ClientStorageRequest, opts ...grpc.CallOption) (*ClientStorageResponse, error)
	UpdateDataShardNotice(ctx context.Context, in *ClientUpdDSRequest, opts ...grpc.CallOption) (*ClientUpdDSResponse, error)
	ACRegisterSN(ctx context.Context, in *ACRegistSNRequest, opts ...grpc.CallOption) (*ACRegistSNResponse, error)
	PreAuditSN(ctx context.Context, in *PASNRequest, opts ...grpc.CallOption) (*PASNResponse, error)
	GetAggPosSN(ctx context.Context, in *GAPSNRequest, opts ...grpc.CallOption) (*GAPSNResponse, error)
}

type sNACServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewSNACServiceClient(cc grpc.ClientConnInterface) SNACServiceClient {
	return &sNACServiceClient{cc}
}

func (c *sNACServiceClient) PutDataShardNotice(ctx context.Context, in *ClientStorageRequest, opts ...grpc.CallOption) (*ClientStorageResponse, error) {
	out := new(ClientStorageResponse)
	err := c.cc.Invoke(ctx, "/proto.SNACService/PutDataShardNotice", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *sNACServiceClient) UpdateDataShardNotice(ctx context.Context, in *ClientUpdDSRequest, opts ...grpc.CallOption) (*ClientUpdDSResponse, error) {
	out := new(ClientUpdDSResponse)
	err := c.cc.Invoke(ctx, "/proto.SNACService/UpdateDataShardNotice", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *sNACServiceClient) ACRegisterSN(ctx context.Context, in *ACRegistSNRequest, opts ...grpc.CallOption) (*ACRegistSNResponse, error) {
	out := new(ACRegistSNResponse)
	err := c.cc.Invoke(ctx, "/proto.SNACService/ACRegisterSN", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *sNACServiceClient) PreAuditSN(ctx context.Context, in *PASNRequest, opts ...grpc.CallOption) (*PASNResponse, error) {
	out := new(PASNResponse)
	err := c.cc.Invoke(ctx, "/proto.SNACService/PreAuditSN", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *sNACServiceClient) GetAggPosSN(ctx context.Context, in *GAPSNRequest, opts ...grpc.CallOption) (*GAPSNResponse, error) {
	out := new(GAPSNResponse)
	err := c.cc.Invoke(ctx, "/proto.SNACService/GetAggPosSN", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// SNACServiceServer is the server API for SNACService service.
// All implementations must embed UnimplementedSNACServiceServer
// for forward compatibility
type SNACServiceServer interface {
	PutDataShardNotice(context.Context, *ClientStorageRequest) (*ClientStorageResponse, error)
	UpdateDataShardNotice(context.Context, *ClientUpdDSRequest) (*ClientUpdDSResponse, error)
	ACRegisterSN(context.Context, *ACRegistSNRequest) (*ACRegistSNResponse, error)
	PreAuditSN(context.Context, *PASNRequest) (*PASNResponse, error)
	GetAggPosSN(context.Context, *GAPSNRequest) (*GAPSNResponse, error)
	mustEmbedUnimplementedSNACServiceServer()
}

// UnimplementedSNACServiceServer must be embedded to have forward compatible implementations.
type UnimplementedSNACServiceServer struct {
}

func (UnimplementedSNACServiceServer) PutDataShardNotice(context.Context, *ClientStorageRequest) (*ClientStorageResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method PutDataShardNotice not implemented")
}
func (UnimplementedSNACServiceServer) UpdateDataShardNotice(context.Context, *ClientUpdDSRequest) (*ClientUpdDSResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method UpdateDataShardNotice not implemented")
}
func (UnimplementedSNACServiceServer) ACRegisterSN(context.Context, *ACRegistSNRequest) (*ACRegistSNResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ACRegisterSN not implemented")
}
func (UnimplementedSNACServiceServer) PreAuditSN(context.Context, *PASNRequest) (*PASNResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method PreAuditSN not implemented")
}
func (UnimplementedSNACServiceServer) GetAggPosSN(context.Context, *GAPSNRequest) (*GAPSNResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetAggPosSN not implemented")
}
func (UnimplementedSNACServiceServer) mustEmbedUnimplementedSNACServiceServer() {}

// UnsafeSNACServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to SNACServiceServer will
// result in compilation errors.
type UnsafeSNACServiceServer interface {
	mustEmbedUnimplementedSNACServiceServer()
}

func RegisterSNACServiceServer(s grpc.ServiceRegistrar, srv SNACServiceServer) {
	s.RegisterService(&SNACService_ServiceDesc, srv)
}

func _SNACService_PutDataShardNotice_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ClientStorageRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SNACServiceServer).PutDataShardNotice(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/proto.SNACService/PutDataShardNotice",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SNACServiceServer).PutDataShardNotice(ctx, req.(*ClientStorageRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _SNACService_UpdateDataShardNotice_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ClientUpdDSRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SNACServiceServer).UpdateDataShardNotice(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/proto.SNACService/UpdateDataShardNotice",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SNACServiceServer).UpdateDataShardNotice(ctx, req.(*ClientUpdDSRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _SNACService_ACRegisterSN_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ACRegistSNRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SNACServiceServer).ACRegisterSN(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/proto.SNACService/ACRegisterSN",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SNACServiceServer).ACRegisterSN(ctx, req.(*ACRegistSNRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _SNACService_PreAuditSN_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(PASNRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SNACServiceServer).PreAuditSN(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/proto.SNACService/PreAuditSN",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SNACServiceServer).PreAuditSN(ctx, req.(*PASNRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _SNACService_GetAggPosSN_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GAPSNRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SNACServiceServer).GetAggPosSN(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/proto.SNACService/GetAggPosSN",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SNACServiceServer).GetAggPosSN(ctx, req.(*GAPSNRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// SNACService_ServiceDesc is the grpc.ServiceDesc for SNACService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var SNACService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "proto.SNACService",
	HandlerType: (*SNACServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "PutDataShardNotice",
			Handler:    _SNACService_PutDataShardNotice_Handler,
		},
		{
			MethodName: "UpdateDataShardNotice",
			Handler:    _SNACService_UpdateDataShardNotice_Handler,
		},
		{
			MethodName: "ACRegisterSN",
			Handler:    _SNACService_ACRegisterSN_Handler,
		},
		{
			MethodName: "PreAuditSN",
			Handler:    _SNACService_PreAuditSN_Handler,
		},
		{
			MethodName: "GetAggPosSN",
			Handler:    _SNACService_GetAggPosSN_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "ac_sn.proto",
}
