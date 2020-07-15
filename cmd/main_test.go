/* Sample Golang Micro-Servcie
  built with go-kit and has 3 below components,
  transport - Supports REST and gRPC and does request validation.
  endpoints - Wraps Service Layer and provides non business logging,instrumentation along with
			  rate limiting,circuit breaker functionality.
  service   - Implements Actual business logic. Support synchronous and asynchronous requests and
			  does business specific instrumentation and logging.

  Repository - Postgres Database is used as backend and all DB logic is abstracted in repo package.
  Non-Functional logic are implemented as middelwares e.g. logging, error handling in each major component.

  uses logging, error handling and config package from other git repos, along with few third party packages.
*/

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Sainarasimhan/sample/pb"
	"github.com/Sainarasimhan/sample/pkg/endpoints"
	repo "github.com/Sainarasimhan/sample/pkg/repository"
	"github.com/Sainarasimhan/sample/pkg/service"
	"github.com/Sainarasimhan/sample/pkg/transport"

	svclog "github.com/Sainarasimhan/Logger"
	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.opentelemetry.io/otel/api/global"
	"go.opentelemetry.io/otel/api/kv"
	"go.opentelemetry.io/otel/api/metric"
	"go.opentelemetry.io/otel/api/trace"
	metricstdout "go.opentelemetry.io/otel/exporters/metric/stdout"
	"go.opentelemetry.io/otel/exporters/trace/stdout"
	"go.opentelemetry.io/otel/sdk/metric/controller/push"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

var (
	otTracer           trace.Tracer
	reqCreate, reqList metric.BoundInt64Counter
	reqLatency         metric.Float64ValueRecorder
	lg                 *svclog.Logger
)

func init() {

	// Setup Logger, tracers and metric instruments
	lg = svclog.New(svclog.Cfg{Level: svclog.LevelDebug, Prefix: "test"})

	// Create stdout exporter to be able to retrieve the collected spans.
	exporter, err := stdout.NewExporter(stdout.Options{
		Writer:      ioutil.Discard,
		PrettyPrint: false})
	if err != nil {
		log.Fatal(err)
	}

	tp, err := sdktrace.NewProvider(sdktrace.WithConfig(sdktrace.Config{DefaultSampler: sdktrace.AlwaysSample()}),
		sdktrace.WithSyncer(exporter))
	if err != nil {
		log.Fatal(err)
	}

	global.SetTraceProvider(tp)
	otTracer = global.Tracer("SampleSvc")

	// stdout metrics exporter
	_, err = metricstdout.InstallNewPipeline(metricstdout.Config{
		Writer:      ioutil.Discard,
		Quantiles:   []float64{0.5, 0.9, 0.99},
		PrettyPrint: true,
	}, push.WithPeriod(60*time.Second))
	if err != nil {
		lg.Printf("failed to initialize metric stdout exporter %v", err)
	}

	// setup metric instruments
	if reqCreateCnt, err := global.Meter("sample").NewInt64Counter("Sample_Svc Create_Requests"); err != nil {
		lg.Error("Action", "MetricsCreation")("Error creating Metrics %s", err.Error())
	} else {
		reqCreate = reqCreateCnt.Bind(kv.String("request", "createsvc"))
		//defer reqCreateSvc.Unbind()
	}
	if reqtListCnt, err := global.Meter("sample").NewInt64Counter("Sample_Svc List_Requests"); err != nil {
		lg.Error("Action", "MetricsCreation")("Error creating Metrics %s", err.Error())
	} else {
		reqList = reqtListCnt.Bind(kv.String("request", "Listsvc"))
		//defer reqListSvc.Unbind()
	}

	if reqLatency, err = global.Meter("sample").NewFloat64ValueRecorder("Request Latency"); err != nil {
		lg.Error("Action", "MetricsCreation")("Error creating Metrics %s", err.Error())
	}
}

func TestHTTP(t *testing.T) {
	tests := []struct {
		name   string
		method string
		url    string
		req    interface{}
		code   int
		resp   interface{}
	}{
		{
			"Invalid Args",
			http.MethodPost,
			"/create",
			pb.CreateRequest{
				ID:     1234,
				Param1: "Test",
			},
			http.StatusBadRequest,
			nil},
		{
			"Create Request",
			http.MethodPost,
			"/create",
			pb.CreateRequest{
				ID:      1234,
				Param1:  "Param1",
				Param2:  "Param2",
				Param3:  "Param3",
				TransID: "Trans-TestID",
			},
			http.StatusOK,
			endpoints.CreateResponse{Err: nil},
		},
		{
			"Async Create Request",
			http.MethodPost,
			"/createAsync",
			pb.CreateRequest{
				ID:      12345,
				Param1:  "Param1",
				Param2:  "Param2",
				Param3:  "Param3",
				TransID: "Trans-TestID",
			},
			http.StatusOK,
			endpoints.CreateResponse{Err: nil},
		},
		{
			"List Request",
			http.MethodGet,
			"/list?id=1234",
			pb.ListRequest{
				ID:      1234,
				TransID: "ListTrans",
			},
			http.StatusOK,
			struct {
				Dtl []*pb.Detail `json:"Dtl"`
			}{
				Dtl: []*pb.Detail{
					{
						ID:     1234,
						Param1: "Param1",
						Param2: "Param2",
						Param3: "Param3",
					}}},
		},
	}

	// setup http handler with sample service
	var (
		db        testRepo
		svc, _    = service.New(lg, &db, service.SetCounters(reqCreate, reqList))
		e         = endpoints.New(svc, lg, reqLatency)
		httpHndlr = transport.NewHTTPHandler(e, otTracer)
	)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Form HTTP request
			body, _ := json.Marshal(tt.req)
			rr, _ := http.NewRequest(tt.method, tt.url, bytes.NewReader(body))
			if tt.method == http.MethodGet {
				lr, _ := tt.req.(pb.ListRequest)
				rr.URL.ForceQuery = true
				rr.Header.Set("TransID", lr.GetTransID())
			}

			// Trigger HTTP Serve with recorder
			rec := httptest.NewRecorder()
			httpHndlr.ServeHTTP(rec, rr)

			// Validate response
			if rec.Code != tt.code {
				t.Errorf("Invalid Response Code, want %d, have %d %v", tt.code, rec.Code, rec.Body)
			}

			//Validate response body on successful casae
			if rec.Code == http.StatusOK {
				exp, _ := json.Marshal(tt.resp)
				exp = append(exp, []byte("\n")...)
				if !bytes.Equal(exp, rec.Body.Bytes()) {
					t.Errorf("Invalid Response Body, have %+v, want %+v", rec.Body, tt.resp)
				}
			}
		})
	}
}

func TestGRPC(t *testing.T) {
	tests := []struct {
		name   string
		method string
		req    interface{}
		code   codes.Code
		resp   interface{}
	}{
		{
			"Invalid Args",
			"Create",
			pb.CreateRequest{
				ID:     1234,
				Param1: "Test",
			},
			codes.InvalidArgument,
			empty.Empty{}},
		{
			"Create Request",
			"Create",
			pb.CreateRequest{
				ID:      1234,
				Param1:  "Param1",
				Param2:  "Param2",
				Param3:  "Param3",
				TransID: "Trans-TestID",
			},
			codes.OK,
			empty.Empty{},
		},
		{
			"List Request",
			"List",
			pb.ListRequest{
				ID:      1234,
				TransID: "ListTrans",
			},
			codes.OK,
			&pb.Details{
				Dtl: []*pb.Detail{
					{
						ID:     1234,
						Param1: "Param1",
						Param2: "Param2",
						Param3: "Param3",
					}}},
		},
	}

	// setup http handler with sample service
	var (
		db      testRepo
		svc, _  = service.New(lg, &db, service.SetCounters(reqCreate, reqList))
		e       = endpoints.New(svc, lg, reqLatency)
		grpcSrv = transport.NewGRPCServer(e)
	)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				err  error
				resp interface{}
			)
			// Form grpc Request
			if tt.method == "Create" || tt.method == "CreateAsync" {

				req, _ := tt.req.(pb.CreateRequest)
				resp, err = grpcSrv.Create(context.Background(), &req)
				var ok bool
				if resp, ok = resp.(*empty.Empty); !ok {
					t.Errorf("Invalid Response Type, have %+v, want %+v", resp, tt.resp)
				}

				// Validate response
				st, _ := status.FromError(err)
				if st.Code() != tt.code {
					t.Errorf("Invalid Response Code, want %d, have %d %v", tt.code, st.Code(), st)
				}

			} else if tt.method == "List" || tt.method == "ListSTream" { //TODO ListStream to be tested
				req, _ := tt.req.(pb.ListRequest)
				resp, err = grpcSrv.List(context.Background(), &req)
				resp, ok := resp.(*pb.Details)
				if !ok {
					t.Errorf("Invalid Response Type, have %+v, want %+v", resp, tt.resp)
				}

				// Validate response
				st, _ := status.FromError(err)
				if st.Code() != tt.code {
					t.Errorf("Invalid Response Code, want %d, have %d %v", tt.code, st.Code(), st)
				}

				//Validate response on successful casae
				if st.Code() == codes.OK {
					expResp, _ := tt.resp.(*pb.Details)
					if len(resp.Dtl) != len(expResp.Dtl) {
						t.Errorf("Invalid Response Body, have %+v, want %+v", resp, tt.resp)
					}
				}
			}
		})
	}
}

func BenchmarkHTTP(b *testing.B) {
	tests := []struct {
		name   string
		method string
		url    string
		req    interface{}
		code   int
		resp   interface{}
	}{
		{
			"Create Request",
			http.MethodPost,
			"/create",
			pb.CreateRequest{
				ID:      1234,
				Param1:  "Param1",
				Param2:  "Param2",
				Param3:  "Param3",
				TransID: "Trans-TestID",
			},
			http.StatusOK,
			endpoints.CreateResponse{Err: nil},
		},
		{
			"List Request",
			http.MethodGet,
			"/list?id=1234",
			pb.ListRequest{
				ID:      1234,
				TransID: "ListTrans",
			},
			http.StatusOK,
			struct {
				Dtl []*pb.Detail `json:"Dtl"`
			}{
				Dtl: []*pb.Detail{
					{
						ID:     1234,
						Param1: "Param1",
						Param2: "Param2",
						Param3: "Param3",
					}}},
		},
	}

	// setup http handler with sample service
	var (
		db     testRepo
		tmplg  = svclog.New(svclog.Cfg{Level: svclog.LevelError})
		svc, _ = service.New(tmplg, &db, service.SetCounters(reqCreate, reqList))
		e      = endpoints.Endpoints{
			Create:      endpoints.MakeCreateEndpoint(svc),
			CreateAsync: endpoints.MakeCreateAsyncEndpoint(svc),
			List:        endpoints.MakeListEndpoint(svc),
		}
		httpHndlr = transport.NewHTTPHandler(e, otTracer)
	)

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			// Form HTTP request
			// Trigger HTTP Serve with recorder
			for i := 0; i < b.N; i++ {
				body, _ := json.Marshal(tt.req)
				rr, _ := http.NewRequest(tt.method, tt.url, bytes.NewReader(body))
				if tt.method == http.MethodGet {
					lr, _ := tt.req.(pb.ListRequest)
					rr.URL.ForceQuery = true
					rr.Header.Set("TransID", lr.GetTransID())
				}
				rec := httptest.NewRecorder()
				httpHndlr.ServeHTTP(rec, rr)
			}

		})
	}
}

func BenchmarkGRPC(b *testing.B) {
	tests := []struct {
		name   string
		method string
		req    interface{}
		code   codes.Code
		resp   interface{}
	}{
		{
			"Create Request",
			"Create",
			pb.CreateRequest{
				ID:      1234,
				Param1:  "Param1",
				Param2:  "Param2",
				Param3:  "Param3",
				TransID: "Trans-TestID",
			},
			codes.OK,
			empty.Empty{},
		},
		{
			"List Request",
			"List",
			pb.ListRequest{
				ID:      1234,
				TransID: "ListTrans",
			},
			codes.OK,
			&pb.Details{
				Dtl: []*pb.Detail{
					{
						ID:     1234,
						Param1: "Param1",
						Param2: "Param2",
						Param3: "Param3",
					}}},
		},
	}

	// setup http handler with sample service
	var (
		db     testRepo
		tmplg  = svclog.New(svclog.Cfg{Level: svclog.LevelError})
		svc, _ = service.New(tmplg, &db, service.SetCounters(reqCreate, reqList))
		e      = endpoints.Endpoints{
			Create: endpoints.MakeCreateEndpoint(svc),
			List:   endpoints.MakeListEndpoint(svc),
		}
		grpcSrv = transport.NewGRPCServer(e)
	)

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				// Form grpc Request
				if tt.method == "Create" || tt.method == "CreateAsync" {

					req, _ := tt.req.(pb.CreateRequest)
					grpcSrv.Create(context.Background(), &req)

				} else if tt.method == "List" || tt.method == "ListSTream" { //TODO ListStream to be tested
					req, _ := tt.req.(pb.ListRequest)
					resp, _ := grpcSrv.List(context.Background(), &req)
					fmt.Println(len(resp.Dtl))
				}
			}
		})
	}
}

// test implementation of repo layer
type testRepo []repo.Details

func (t *testRepo) Insert(_ context.Context, req repo.Request) (repo.Response, error) {
	*t = append(*t, repo.Details{
		ID:     req.ID,
		Param1: req.Param1,
		Param2: req.Param2,
		Param3: req.Param3,
	})
	return repo.Response{}, nil

}
func (t *testRepo) Update(_ context.Context, req repo.Request) error {
	for i, d := range *t {
		if d.ID == req.ID {
			newDtl := repo.Details{
				ID:     req.ID,
				Param1: req.Param1,
				Param2: req.Param2,
				Param3: req.Param3,
			}
			(*t)[i] = newDtl
			break
		}
	}
	return nil
}
func (t *testRepo) List(_ context.Context, req repo.Request) ([]repo.Details, error) {
	// Return details as present in testcase's expected body
	resp := []repo.Details{}
	for _, d := range *t {
		if d.ID == req.ID {
			resp = append(resp, d)
		}
	}
	return resp, nil
}
func (t *testRepo) Delete(_ context.Context, req repo.Request) error {
	for i, d := range *t {
		if d.ID == req.ID {
			*t = append((*t)[:i], (*t)[(i+1):]...)
		}
	}
	return nil
}
func (t *testRepo) Close() {
	// Nothing
}
