package transport

import (
	"context"
	"net/http"
	"strconv"

	log "github.com/Sainarasimhan/Logger"
	"github.com/Sainarasimhan/sample/pb"
	newlog "github.com/Sainarasimhan/sample/pkg/log"
)

// Functionality to setup request context for logs

const (
	HTTP   = "Http"
	GRPC   = "gRPC"
	PUBSUB = "PubSub"
	CREATE = "Create"
	LIST   = "List"
)

func SetHTTPLogContext(ctx context.Context, r *http.Request) context.Context {
	return newlog.NewContext(ctx, &newlog.MsgStruct{
		Transport: HTTP,
		// Other params to be setup while decoding HTTP request
	})
}

func UpdateHTTPLogContext(ctx context.Context, id string, evt string) {
	s, ok := newlog.FromContext(ctx)
	if true == ok {
		s.Event = evt
		s.ID = id
	}
}

func SetGRPCLogContext(ctx context.Context, id string, evt string) context.Context {
	return newlog.NewContext(ctx, &newlog.MsgStruct{
		Transport: GRPC,
		ID:        id,
		Event:     evt,
	})
}

func SetPubSubLogContext(ctx context.Context, id string, evt string) context.Context {
	return newlog.NewContext(ctx, &newlog.MsgStruct{
		Transport: PUBSUB,
		ID:        id,
		Event:     evt,
	})
}

// New Logger created for each req at transport level with request specific context and
// passed to endpoint/service.
// Logic to Add request specific Logger - To Be Analyzed
// Keeping it for backup
//GRPCLogger -- Returns Logger for GRPC
func GRPCLogger(req interface{}) *log.Logger {
	return newLogger("GRPC", req)
}

//HTTPLogger -- Returns Lggger for HTTP
func HTTPLogger(req interface{}) *log.Logger {
	return newLogger("HTTP", req)
}

//PubSubLogger -- Returns Logger for PubSub
func PubSubLogger(req interface{}) *log.Logger {
	return newLogger("PubSub", req)
}

func newLogger(transport string, req interface{}) *log.Logger {

	// Add Prefix as req
	prefix := "SampleSvc" + "tr=" + transport + " "

	switch req.(type) {
	case *pb.CreateRequest:
		prefix += createPrefix(req.(*pb.CreateRequest))
	case *pb.ListRequest:
		prefix += listPrefix(req.(*pb.ListRequest))
	}

	// Create new Logger for the request
	lg := log.New(log.Cfg{
		Flags:  log.LstdFlags,
		Prefix: prefix,
		Level:  log.LevelDebug,
	})

	return lg
}

func createPrefix(req *pb.CreateRequest) string {
	return "req=Create TID=" + req.GetTransID() + "ID=" + strconv.Itoa(int(req.GetID())) // Add Contexts as required
}

func listPrefix(req *pb.ListRequest) string {
	return "req=List TID=" + req.GetTransID() + "ID=" + strconv.Itoa(int(req.GetID())) // Add Contexts as required
}
