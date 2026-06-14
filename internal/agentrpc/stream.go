package agentrpc

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/structpb"
)

const AgentServiceFullName = "aiops.agent.v1.AgentService"

type AgentServiceClient interface {
	Connect(ctx context.Context, opts ...grpc.CallOption) (AgentServiceConnectClient, error)
}

type AgentServiceConnectClient interface {
	Send(*structpb.Struct) error
	Recv() (*structpb.Struct, error)
	grpc.ClientStream
}

type agentServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewAgentServiceClient(cc grpc.ClientConnInterface) AgentServiceClient {
	return &agentServiceClient{cc: cc}
}

func (c *agentServiceClient) Connect(ctx context.Context, opts ...grpc.CallOption) (AgentServiceConnectClient, error) {
	stream, err := c.cc.NewStream(ctx, &AgentService_ServiceDesc.Streams[0], "/"+AgentServiceFullName+"/Connect", opts...)
	if err != nil {
		return nil, err
	}
	return &agentServiceConnectClient{ClientStream: stream}, nil
}

type agentServiceConnectClient struct {
	grpc.ClientStream
}

func (c *agentServiceConnectClient) Send(msg *structpb.Struct) error {
	return c.ClientStream.SendMsg(msg)
}

func (c *agentServiceConnectClient) Recv() (*structpb.Struct, error) {
	msg := new(structpb.Struct)
	if err := c.ClientStream.RecvMsg(msg); err != nil {
		return nil, err
	}
	return msg, nil
}

type AgentServiceServer interface {
	Connect(AgentServiceConnectServer) error
}

type AgentServiceConnectServer interface {
	Send(*structpb.Struct) error
	Recv() (*structpb.Struct, error)
	grpc.ServerStream
}

func RegisterAgentServiceServer(s grpc.ServiceRegistrar, srv AgentServiceServer) {
	s.RegisterService(&AgentService_ServiceDesc, srv)
}

func _AgentService_Connect_Handler(srv any, stream grpc.ServerStream) error {
	return srv.(AgentServiceServer).Connect(&agentServiceConnectServer{ServerStream: stream})
}

type agentServiceConnectServer struct {
	grpc.ServerStream
}

func (s *agentServiceConnectServer) Send(msg *structpb.Struct) error {
	return s.ServerStream.SendMsg(msg)
}

func (s *agentServiceConnectServer) Recv() (*structpb.Struct, error) {
	msg := new(structpb.Struct)
	if err := s.ServerStream.RecvMsg(msg); err != nil {
		return nil, err
	}
	return msg, nil
}

var AgentService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: AgentServiceFullName,
	HandlerType: (*AgentServiceServer)(nil),
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "Connect",
			Handler:       _AgentService_Connect_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "proto/agent.proto",
}
