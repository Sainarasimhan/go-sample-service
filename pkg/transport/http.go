package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/Sainarasimhan/sample/pb"
	"github.com/Sainarasimhan/sample/pkg/endpoints"

	svcerr "github.com/Sainarasimhan/go-error/err"
	httptransport "github.com/go-kit/kit/transport/http"
	"github.com/gorilla/mux"
	otelhttp "go.opentelemetry.io/contrib/instrumentation/net/http/httptrace"
	"go.opentelemetry.io/otel/api/correlation"
	"go.opentelemetry.io/otel/api/trace"
	otelcode "go.opentelemetry.io/otel/codes"
)

const (
	//Timeout - max Time for handling a Http request
	Timeout time.Duration = time.Second * 10
	//TimeoutError - Error to send out, when max time reached on handling http request
	TimeoutError = "Request Handling Timeout"
)

// NewHTTPHandler - Creates new HTTP Handler
// sets up opentelemtry tracer as part of HTTP Handler
// Converts any error to standard error structure
func NewHTTPHandler(endpoints endpoints.Endpoints, tracer trace.Tracer) http.Handler {
	options := []httptransport.ServerOption{
		HTTPOpenTMTrace(tracer),
		httptransport.ServerErrorEncoder(errorEncoder),
		httptransport.ServerBefore(SetHTTPLogContext),
	}
	m := mux.NewRouter()
	m.Methods(http.MethodPost).Path("/create").
		Handler(httptransport.NewServer(
			endpoints.Create,
			DecodeCreateRequest,
			EncodeCreateResponse,
			options...),
		)
	m.Methods(http.MethodPost).Path("/createAsync").
		Handler(httptransport.NewServer(
			endpoints.CreateAsync,
			DecodeCreateRequest,
			EncodeCreateResponse,
			options...),
		)
	m.Methods(http.MethodGet).Path("/list").
		Queries("id", "{id}").
		Handler(httptransport.NewServer(
			endpoints.List,
			DecodeListRequest,
			EncodeListResponse,
			options...))
	return m
}

// DecodeCreateRequest - Decodes Json from request body and validates the requests
func DecodeCreateRequest(ctx context.Context, r *http.Request) (interface{}, error) {
	var (
		req endpoints.CreateRequest
		msg = &pb.CreateRequest{}
	)
	err := json.NewDecoder(r.Body).Decode(msg)
	if err != nil {
		return req, svcerr.InvalidArgs("Invalid Request Body")
	}
	if err = ValidateRequest(msg); err != nil {
		return req, err
	}

	//Set Log Context?
	UpdateHTTPLogContext(ctx, msg.GetTransID(), CREATE)

	//Assign the Protobuf message to request
	req.Cr.CreateRequest = msg

	return req, err
}

//EncodeCreateResponse - standard encoding for create
func EncodeCreateResponse(_ context.Context, w http.ResponseWriter, response interface{}) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(response)
}

// DecodeListRequest - Decodes List GET request
func DecodeListRequest(ctx context.Context, r *http.Request) (interface{}, error) {
	var (
		req endpoints.ListRequest
		msg = &pb.ListRequest{}
	)
	id, err := strconv.Atoi(r.URL.Query().Get("id"))
	if err != nil {
		return req, svcerr.InvalidArgs("Invalid Query Params")
	}
	msg.ID = int32(id)
	msg.TransID = r.Header.Get("TransID")
	if err = ValidateRequest(msg); err != nil {
		return req, err
	}

	//Set Log Context?
	UpdateHTTPLogContext(ctx, msg.GetTransID(), LIST)
	//Assign the Protobuf message to request
	req.Lr.ListRequest = msg
	return req, err
}

// EncodeListResponse .- Stdrd Encoding for List
func EncodeListResponse(_ context.Context, w http.ResponseWriter, response interface{}) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(response)
}

// sends uniform error structure
func errorEncoder(ctx context.Context, err error, w http.ResponseWriter) {
	//Updating to work with generic error codes
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	se := svcerr.ConvHTTP(err)
	w.WriteHeader(se.Rest.Code)
	json.NewEncoder(w).Encode(se)

	// Set error code in the span
	if span := trace.SpanFromContext(ctx); span != nil {
		var c otelcode.Code = otelcode.Code(int(svcerr.Code(err)))
		span.SetStatus(c, http.StatusText(se.Rest.Code))
	}

}

//HTTPOpenTMTrace - Function to setup Opentelemtry tracing for http request
func HTTPOpenTMTrace(tr trace.Tracer) httptransport.ServerOption {
	/*config := tracerOptions{ TODO Setup options
		tags:      make(map[string]string),
		name:      "",
		logger:    log.NewNopLogger(),
		propagate: true,
	}

	for _, option := range options {
		option(&config)
	}*/

	serverBefore := httptransport.ServerBefore(
		func(ctx context.Context, req *http.Request) context.Context {
			attrs, entries, spanCtx := otelhttp.Extract(req.Context(), req)

			req = req.WithContext(correlation.ContextWithMap(req.Context(), correlation.NewMap(correlation.MapUpdate{
				MultiKV: entries,
			})))

			ctx, span := tr.Start(
				trace.ContextWithRemoteSpanContext(req.Context(), spanCtx),
				"sample service Http call",
				trace.WithAttributes(attrs...),
			)
			span.AddEvent(ctx, "handling http request")
			return ctx
		},
	)

	serverAfter := httptransport.ServerAfter(
		func(ctx context.Context, _ http.ResponseWriter) context.Context {
			if span := trace.SpanFromContext(ctx); span != nil {
				span.End()
			}
			return ctx
		},
	)

	serverFinalizer := httptransport.ServerFinalizer(
		func(ctx context.Context, code int, r *http.Request) {
			if span := trace.SpanFromContext(ctx); span != nil {
				span.End()
			}
		},
	)

	return func(s *httptransport.Server) {
		serverBefore(s)
		serverAfter(s)
		serverFinalizer(s)
	}
}
