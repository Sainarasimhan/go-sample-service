package endpoints

import (
	"context"
	"fmt"
	"os"
	"runtime/pprof"
	"sample/pkg/service"
	"time"

	log "github.com/Sainarasimhan/Logger"
	"github.com/go-kit/kit/circuitbreaker"
	"github.com/go-kit/kit/endpoint"
	"github.com/go-kit/kit/ratelimit"
	"github.com/sony/gobreaker"
	"go.opentelemetry.io/otel/api/metric"
	"golang.org/x/time/rate"
)

/*Package contains non-business Logic, such as
- Req/Resp Logs,
- Metrics,
- Tracing
- Rate Limiting/Threshold
- Circuit Breakers
*/

// CreateRequest - Request for create
type CreateRequest struct {
	Cr service.CreateRequest
}

// CreateResponse - Endpoint response for create
type CreateResponse struct {
	Err error `json:"Err"`
}

// ListRequest - Endpoint wrapper for List
type ListRequest struct {
	Lr service.ListRequest
}

// ListResponse - Endpoint wrapper for List response
type ListResponse struct {
	D   service.Details
	Err error `json:"Err"`
}

// New - Creates endpoints for create,list with all the middlewares setup
// Logging Middleware - to log requests, time taken.
// Instumenting Middleware - to capture request latency in metric instruments
// Circuit breaker middleware - CB functionality with default settings.
// Rate Limiting Middleware - currently allows max 10 requests per sec. No Throttling setup atm
func New(svc service.Service, lg *log.Logger, latency metric.Float64ValueRecorder) Endpoints { //Pass Tracer and Metrics Objects

	createEndpoint := MakeCreateEndpoint(svc)
	createEndpoint = LoggingMiddleware(lg)(createEndpoint)
	createEndpoint = InstrumentingMiddelware(latency)(createEndpoint)
	createEndpoint = circuitbreaker.Gobreaker(gobreaker.NewCircuitBreaker(gobreaker.Settings{}))(createEndpoint)
	createEndpoint = ratelimit.NewErroringLimiter(rate.NewLimiter(rate.Every(time.Second), 10))(createEndpoint) //Max 10 requests?

	crAsyncEndpoint := MakeCreateAsyncEndpoint(svc)
	crAsyncEndpoint = LoggingMiddleware(lg)(crAsyncEndpoint)
	crAsyncEndpoint = InstrumentingMiddelware(latency)(crAsyncEndpoint)
	crAsyncEndpoint = circuitbreaker.Gobreaker(gobreaker.NewCircuitBreaker(gobreaker.Settings{}))(crAsyncEndpoint)
	crAsyncEndpoint = ratelimit.NewErroringLimiter(rate.NewLimiter(rate.Every(time.Second), 10))(crAsyncEndpoint) //Max 10 requests?

	listEndpoint := MakeListEndpoint(svc)
	listEndpoint = LoggingMiddleware(lg)(listEndpoint)
	listEndpoint = InstrumentingMiddelware(latency)(listEndpoint)
	listEndpoint = circuitbreaker.Gobreaker(gobreaker.NewCircuitBreaker(gobreaker.Settings{}))(listEndpoint)
	listEndpoint = ratelimit.NewErroringLimiter(rate.NewLimiter(rate.Every(time.Second), 200))(listEndpoint)

	return Endpoints{
		Create:      createEndpoint,
		CreateAsync: crAsyncEndpoint,
		List:        listEndpoint,
	}
}

// MakeCreateEndpoint - Creates endpoing for create
func MakeCreateEndpoint(s service.Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(CreateRequest)
		err := s.Create(ctx, req.Cr)
		return CreateResponse{Err: err}, err
	}
}

// MakeCreateAsyncEndpoint - Creates endpoing for Async create
func MakeCreateAsyncEndpoint(s service.Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		var (
			req = request.(CreateRequest)
			err error
		)
		err = s.CreateAsync(ctx, req.Cr)
		return CreateResponse{Err: err}, err
	}
}

// MakeListEndpoint - Creates List endpoint
func MakeListEndpoint(s service.Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(ListRequest)
		D, err := s.List(ctx, req.Lr)
		//return ListResponse{D: D, Err: err}, err
		return D.Details, err
	}
}

//Endpoints - holds the endpoints
type Endpoints struct {
	Create      endpoint.Endpoint
	CreateAsync endpoint.Endpoint
	List        endpoint.Endpoint
}

// helper func to capture memprofile, currently not used
func memProfile(memprofile string) {
	if memprofile != "" {
		fmt.Println("Doing memprofile")
		f, err := os.Create(memprofile)
		if err != nil {
			fmt.Println("could not create memory profile: ", err)
		}
		defer f.Close()
		//runtime.GC() // get up-to-date statistics
		if err := pprof.WriteHeapProfile(f); err != nil {
			fmt.Println("could not write memory profile: ", err)
		}
	}
}
