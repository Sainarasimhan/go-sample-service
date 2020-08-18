package transport

import (
	"context"

	"github.com/Sainarasimhan/sample/pb"
	"github.com/Sainarasimhan/sample/pkg/endpoints"
	"github.com/Sainarasimhan/sample/pkg/service"

	svcerr "github.com/Sainarasimhan/go-error/err"
	"github.com/golang/protobuf/ptypes/empty"

	grpctransport "github.com/go-kit/kit/transport/grpc"
)

type grpcServer struct {
	create      grpctransport.Handler
	createAsync grpctransport.Handler
	list        grpctransport.Handler
}

// NewGRPCServer makes endpoints available as a gRPC LocationsServer
func NewGRPCServer(endpoints endpoints.Endpoints) pb.SampleServer {

	return &grpcServer{
		create: grpctransport.NewServer(
			endpoints.Create,
			DecodeGRPCCreateRequest,
			EncodeGRPCCreateResponse),
		createAsync: grpctransport.NewServer(
			endpoints.CreateAsync,
			DecodeGRPCCreateRequest,
			EncodeGRPCCreateResponse),
		list: grpctransport.NewServer(
			endpoints.List,
			DecodeGRPCListRequest,
			EncodeGRPCListResponse),
	}
}

// Create - gRPC handler for Create requests
func (g *grpcServer) Create(ctx context.Context, cr *pb.CreateRequest) (*empty.Empty, error) {
	ctx = SetGRPCLogContext(ctx, cr.GetTransID(), CREATE)
	_, _, err := g.create.ServeGRPC(ctx, cr)
	if err != nil {
		return nil, err
	}
	return &empty.Empty{}, nil

}

// List - gRPC handler for List
func (g *grpcServer) List(ctx context.Context, lr *pb.ListRequest) (*pb.Details, error) {
	ctx = SetGRPCLogContext(ctx, lr.GetTransID(), LIST)
	_, resp, err := g.list.ServeGRPC(ctx, lr)
	if err != nil {
		return nil, err
	}
	list := resp.(*pb.Details)
	return list, nil
}

// ListStream - gRPC handler for List stream
func (g *grpcServer) ListStream(lr *pb.ListRequest, strm pb.Sample_ListStreamServer) error {
	ctx := SetGRPCLogContext(context.Background(), lr.GetTransID(), LIST)
	_, resp, err := g.list.ServeGRPC(ctx, lr)
	if err != nil {
		return err
	}
	list := resp.(*pb.Details)
	for _, l := range list.Dtl {
		if err := strm.Send(l); err != nil {
			return err
		}
	}
	return nil
}

// DecodeGRPCCreateRequest -- Decode from gRPC to endpoint request
func DecodeGRPCCreateRequest(_ context.Context, grpcReq interface{}) (interface{}, error) {
	req, ok := grpcReq.(*pb.CreateRequest)
	if !ok {
		return nil, svcerr.InvalidArgs("Invalid gRPC request")
	}
	if err := ValidateRequest(req); err != nil {
		return nil, err
	}
	return endpoints.CreateRequest{
		Cr: service.CreateRequest{
			CreateRequest: req},
	}, nil
}

// DecodeGRPCListRequest -- Decode from gRPC to endpoint request
func DecodeGRPCListRequest(_ context.Context, grpcReq interface{}) (interface{}, error) {
	req, ok := grpcReq.(*pb.ListRequest)
	if !ok {
		return nil, svcerr.InvalidArgs("Invalid gRPC request")
	}
	if err := ValidateRequest(req); err != nil {
		return nil, err
	}

	return endpoints.ListRequest{
		Lr: service.ListRequest{
			ListRequest: req},
	}, nil
}

// EncodeGRPCCreateResponse - stdr Encoding
func EncodeGRPCCreateResponse(_ context.Context, response interface{}) (interface{}, error) {
	return &empty.Empty{}, nil
}

// EncodeGRPCListResponse - Makes sure response is of exptected structure
func EncodeGRPCListResponse(_ context.Context, response interface{}) (interface{}, error) {
	if resp, ok := response.(*pb.Details); ok {
		return resp, nil
	}
	return nil, svcerr.InternalErr("Error encoding gRPC Response")
}
