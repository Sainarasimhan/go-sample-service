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

package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Sainarasimhan/sample/pb"
	"github.com/Sainarasimhan/sample/pkg/endpoints"
	"github.com/Sainarasimhan/sample/pkg/transport"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	httpHndlr http.Handler
	grpcSrv   pb.SampleServer
)

func init() {

	// Get Dependencies and endpoints from Wire Build, ignore error
	deps, _, err := GetBaseDeps()
	if err != nil {
		log.Fatal(err)
	}
	testEndpoint, _, err := GetTestEndpoint()
	if err != nil {
		log.Fatal(err)
	}

	// setup http handler with sample service
	httpHndlr = transport.NewHTTPHandler(testEndpoint, deps.OTTracer)
	// setup GRPC handler with sample service
	grpcSrv = transport.NewGRPCServer(testEndpoint)
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
				ID:     123456,
				Param1: "Test",
			},
			codes.InvalidArgument,
			empty.Empty{}},
		{
			"Create Request",
			"Create",
			pb.CreateRequest{
				ID:      123456,
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
				ID:      123456,
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
