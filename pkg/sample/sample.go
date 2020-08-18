package sample

// Package has all the provider functions of sample service.
// Wire Build is used to build injectors with Provider functions.
import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime/pprof"
	"syscall"
	"time"

	"github.com/Sainarasimhan/sample/pb"
	config "github.com/Sainarasimhan/sample/pkg/cfg"
	"github.com/Sainarasimhan/sample/pkg/endpoints"
	"github.com/Sainarasimhan/sample/pkg/log"
	repo "github.com/Sainarasimhan/sample/pkg/repository"
	"github.com/Sainarasimhan/sample/pkg/service"
	"github.com/Sainarasimhan/sample/pkg/transport"
	"github.com/google/wire"
	"go.opentelemetry.io/otel/api/global"
	"go.opentelemetry.io/otel/api/kv"
	"go.opentelemetry.io/otel/api/trace"
	metricstdout "go.opentelemetry.io/otel/exporters/metric/stdout"
	"go.opentelemetry.io/otel/exporters/trace/stdout"
	"go.opentelemetry.io/otel/plugin/grpctrace"
	"go.opentelemetry.io/otel/sdk/metric/controller/push"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"

	"go.opentelemetry.io/otel/api/metric"
)

var (
	// BaseDeps - Set that provides base dependencies.
	BaseDeps = wire.NewSet(
		ProvideCfg,
		ProvideLog,
		ProvideTracer,
		ProvideMetricInstruments,
		wire.Struct(new(BaseDependencies), "Logger", "Cfg", "MetricInstruments", "OpenTelemetryExporters"),
	)

	// BaseSet - Set providing Sample service/Endpoint for transport handlers to use.
	BaseSet = wire.NewSet(
		ProvideCfg,
		ProvideLog,
		ProvideTracer,
		ProvideMetricInstruments,
		wire.Struct(new(BaseDependencies), "Logger", "Cfg", "MetricInstruments", "OpenTelemetryExporters"),
		ProvideRepo,
		ProvideService,
		ProvideEndpoint,
	)

	// FullSet - Set providing all transport handlers (REST+gRPC+PubSub)
	FullSet = wire.NewSet(
		BaseSet,
		ProvideDebugHndlr,
		ProvideInterruptHandlers,
		ProvideHTTPTransport,
		ProvideGRPCTransport,
		ProvideSubsClient,
		wire.Struct(new(Functions), "HTTPTransport", "GRPCTransport", "DebugTransport", "PubSubTransport", "InterruptHandler"),
		ProvideFunctionsList,
	)

	// HTTPSet - Set providing HTTP Handler
	HTTPSet = wire.NewSet(
		BaseSet,
		ProvideHTTPTransport,
		wire.Struct(new(Functions), "HTTPTransport"),
		ProvideFunctionsList,
	)

	// GRPCSet - Set Providing GRPC Handler
	GRPCSet = wire.NewSet(
		BaseSet,
		ProvideDebugHndlr,
		ProvideInterruptHandlers,
		ProvideGRPCTransport,
		wire.Struct(new(Functions), "GRPCTransport", "DebugTransport", "InterruptHandler"),
		ProvideFunctionsList,
	)

	// ProdSet - Set providing PROD Handlers (HTTP + gRPC)
	ProdSet = wire.NewSet(
		BaseSet,
		ProvideDebugHndlr,
		ProvideInterruptHandlers,
		ProvideHTTPTransport,
		ProvideGRPCTransport,
		wire.Struct(new(Functions), "HTTPTransport", "GRPCTransport", "DebugTransport", "InterruptHandler"),
		ProvideFunctionsList,
	)

	// Add more provider sets as needed
)

// BaseDependencies - Struct holding Basic deps
type BaseDependencies struct {
	log.Logger
	*config.Cfg
	OpenTelemetryExporters
	MetricInstruments
}

// MetricInstruments - Instrumentation Params
type MetricInstruments struct {
	ReqCreateSvc, ReqListSvc metric.BoundInt64Counter
	ReqLatency               metric.Float64ValueRecorder
	DBMetrics                *repo.Metrics
}

// OpenTelemetryExporters - Metrics/Tracing Exporters (OpenTelemetry)
type OpenTelemetryExporters struct {
	OTTracer trace.Tracer
	Pusher   *push.Controller
}

// ProvideCfg - Inits config file and provides back cfg struct
func ProvideCfg() (cfg *config.Cfg, err error) {
	return config.Init()
}

// ProvideLog - Log Provider
func ProvideLog(cfg *config.Cfg) (lg log.Logger) {
	return log.New("Main", cfg.Sample.Log)
}

// ProvideTracer - Returns Tracer and Metrics pusher
// TODO - setup Tracer and Metrics exported based on config
func ProvideTracer(lg log.Logger) (ot OpenTelemetryExporters, cleanup func(), err error) {

	ctx := context.Background()

	// ---------------- Tracer Setup --------------------
	// zipkin exporter can be setup as below
	/*exporter, err := otzipkin.NewExporter(*zipkinURL, "Sample")
	if err != nil {
		lg.Fatal(err)
	}*/

	// For the demonstration, use sdktrace.AlwaysSample sampler to sample all traces.
	// In a production application, use sdktrace.ProbabilitySampler with a desired probability.
	/*tp, err := sdktrace.NewProvider(sdktrace.WithConfig(sdktrace.Config{DefaultSampler: sdktrace.AlwaysSample()}),
		sdktrace.WithBatcher(exporter,
			sdktrace.WithBatchTimeout(5*time.Second),
			sdktrace.WithMaxExportBatchSize(10),
		),
	)*/

	// Create stdout exporter to be able to retrieve the collected spans.
	exporter, err := stdout.NewExporter(stdout.Options{PrettyPrint: true})
	if err != nil {
		lg.Error(ctx, err)
		return
	}

	tp, err := sdktrace.NewProvider(sdktrace.WithConfig(sdktrace.Config{DefaultSampler: sdktrace.ProbabilitySampler(.1)}),
		sdktrace.WithSyncer(exporter))
	if err != nil {
		lg.Error(ctx, err)
		return
	}

	global.SetTraceProvider(tp)
	otTracer := global.Tracer("SampleSvc") // Tracer
	// ---------------- Tracer Setup End --------------------

	// ---------------- Metrics exporter Setup --------------------
	// Prometheus exporter can be setup as below
	/*exporter, err := otPrometheus.InstallNewPipeline(otPrometheus.Config{
		Registerer: prometheus.DefaultRegisterer,
		Gatherer:   prometheus.DefaultGatherer,
	})
	if err != nil {
		lg.Panicf("failed to initialize prometheus exporter %v", err)
	}
	http.HandleFunc("/", exporter.ServeHTTP)
	*/

	// stdout exporter
	pusher, err := metricstdout.InstallNewPipeline(metricstdout.Config{
		Quantiles:   []float64{0.5, 0.9, 0.99},
		PrettyPrint: true,
	}, push.WithPeriod(60*time.Second))
	if err != nil {
		lg.Error(ctx, "failed to initialize metric stdout exporter %v", err)
		return
	}
	// ---------------- Metrics exporter Setup End --------------------

	// return Tracer,Pusher along with cleanup
	ot.OTTracer, ot.Pusher = otTracer, pusher
	cleanup = func() {
		pusher.Stop()
	}
	return
}

// ProvideMetricInstruments - Builds various Metric Instruments.
func ProvideMetricInstruments(lg log.Logger, ot OpenTelemetryExporters) (mi MetricInstruments, cleanup func(), err error) {
	ctx := context.Background()
	// Create Metric Instruments
	if reqCreateCnt, err := global.Meter("sample").NewInt64Counter("Sample_Svc Create_Requests"); err != nil {
		lg.Error(ctx, "Action", "MetricsCreation", "Error creating Metrics %s", err.Error())
	} else {
		mi.ReqCreateSvc = reqCreateCnt.Bind(kv.String("request", "createsvc"))
	}
	if reqtListCnt, err := global.Meter("sample").NewInt64Counter("Sample_Svc List_Requests"); err != nil {
		lg.Error(ctx, "Action", "MetricsCreation", "Error creating Metrics %s", err.Error())
	} else {
		mi.ReqListSvc = reqtListCnt.Bind(kv.String("request", "Listsvc"))
	}

	if mi.ReqLatency, err = global.Meter("sample").NewFloat64ValueRecorder("Request Latency"); err != nil {
		lg.Error(ctx, "Action", "MetricsCreation", "Error creating Metrics %s", err.Error())
	}

	mi.DBMetrics = &repo.Metrics{}
	batchObserver := global.Meter("Sampple-DB").NewBatchObserver(mi.DBMetrics.DoMetrics())
	mi.DBMetrics.OpenCnx, _ = batchObserver.NewInt64ValueObserver("Opne DB Connections")
	mi.DBMetrics.IdleCnx, _ = batchObserver.NewInt64ValueObserver("Idle DB Connections")
	mi.DBMetrics.IdelClosed, _ = batchObserver.NewInt64SumObserver("Idle Close DB Connections")

	lg.Info(ctx, "Setup Metric Instruments - CreateCount, ListCount, ReqLatency, DBMetrics")
	cleanup = func() {
		mi.ReqCreateSvc.Unbind()
		mi.ReqListSvc.Unbind()
	}
	return
}

// ProvideRepo - Builds Repo Layer
func ProvideRepo(deps BaseDependencies) (db repo.Repository, cleanup func(), err error) {
	var (
		ctx    = context.Background()
		dbCfg  = deps.DB
		repolg = log.NewZapLogger("Repo", dbCfg.Log) // Logger for Repo Layer
	)

	db, err = repo.NewPostgres(dbCfg.PostgresStr(), repolg, // Postgres
		repo.SetMaxPostgresConn(dbCfg.MaxConns),
		repo.SetTracer(deps.OTTracer),
		repo.EnableMetrics(deps.DBMetrics))
	if err != nil {
		repolg.Error(ctx, "Error creating repo", err) // Service exits on any error creating repo
		return
	}
	cleanup = func() {
		db.Close()
	}
	return
}

// ProvideService - Builds core Service
func ProvideService(deps BaseDependencies, db repo.Repository) (svc service.Service, err error) {
	// Create Service, Endpoints and Transport Handlers
	// service created is passed to endpoints,
	// endpoints is passed to transport handlers, along with other dependencies.

	var (
		lg = log.New("Svc", deps.Sample.Log)
		// Service options
		opts = []service.Option{
			// Service Options
			service.SetAsyncWorkers(deps.Sample.ConcurrentWrites),
			service.SetCounters(deps.ReqCreateSvc, deps.ReqListSvc),
			/* Publish Client and Error reporting client can be setup as below.
			service.SetPublishEvent(
			transport.NewPubClient(lg, cfg.Sample.PS.PubURL)), //Create Publish Event to push notifications
			service.SetErrorReportingClient(
				service.NewErrorReportingClient(lg, cfg.Sample.ErrRprtPrjID)), //Setup GCP Error reporting client
			*/

		}
	)
	return service.New(lg, db, opts...)
}

// ProvideEndpoint - Provides Endpoint
func ProvideEndpoint(deps BaseDependencies, svc service.Service) (endpoints.Endpoints, error) {
	return endpoints.New(svc, log.New("EndPoint", deps.Sample.Log), deps.ReqLatency), nil
}

// SampleFunction - basic structure of Sample Functionalities.
type SampleFunction struct {
	Enabled  bool
	Start    func() error
	Shutdown func()
}

// Various Sample Functionalities.
type HTTPTransport SampleFunction
type GRPCTransport SampleFunction
type DebugTransport SampleFunction
type PubSubTransport SampleFunction
type InterruptHandler SampleFunction

// Struct to hold diff functionalities.
type Functions struct {
	HTTPTransport
	GRPCTransport
	DebugTransport
	PubSubTransport
	InterruptHandler
}

// Slice of Functionalities
type FunctionsList []SampleFunction

// ProvideHTTPTransport - Builds HTTP Handler
func ProvideHTTPTransport(e endpoints.Endpoints, deps BaseDependencies) (HTTPTransport, error) {
	// The HTTP listener mounts the Go kit HTTP handler we created.
	var (
		ctx         = context.Background()
		httpCfg     = deps.Sample.HTTP
		httpAddress = httpCfg.Address()
		httpHndlr   = http.TimeoutHandler(
			transport.NewHTTPHandler(e, deps.OTTracer),
			transport.Timeout,
			transport.TimeoutError)
	)

	srv := &http.Server{ // Setup server with Timeouts
		Addr:         httpAddress,
		Handler:      httpHndlr,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		TLSConfig: &tls.Config{
			ClientAuth: tls.RequireAndVerifyClientCert,
		},
	}

	startFn := func() error {
		deps.Infow(ctx, "Address", httpAddress, "msg", "starting HTTP server ...")
		if httpCfg.Secure {
			deps.Info(context.Background(), "Am I ")
			return srv.ListenAndServeTLS(httpCfg.TLSCert, httpCfg.TLSKey)
		}
		return srv.ListenAndServe()
	}

	shutDownFn := func() {
		deps.Infow(ctx, "msg", "Shutting Down HTTP Handler")
		sdctx, sdcancel := context.WithTimeout(context.Background(), 60*time.Second)
		// wait for 60 seconds max
		defer sdcancel()
		if srv != nil {
			srv.Shutdown(sdctx)
		}
	}

	return HTTPTransport{
		Enabled:  true,
		Start:    startFn,
		Shutdown: shutDownFn,
	}, nil
}

// ProvideGRPCTransport -- Builds GRPC Transport Handler
func ProvideGRPCTransport(deps BaseDependencies, e endpoints.Endpoints) (GRPCTransport, error) {
	// The gRPC listener mounts the Go kit gRPC server we created.
	var (
		grpcCfg           = deps.Sample.GRPC
		grpcListener, err = net.Listen("tcp", grpcCfg.Address())
		baseServer        *grpc.Server
		grpcHndlr         = transport.NewGRPCServer(e)
		ctx               = context.Background()
	)
	if err != nil {
		deps.Fatal(ctx, "transport gRPC \t", err)
	}
	opts := []grpc.ServerOption{}

	//Add KeepAlive Timeout
	opts = append(opts, grpc.KeepaliveParams(keepalive.ServerParameters{
		MaxConnectionAge:      5 * time.Minute,
		MaxConnectionAgeGrace: 5 * time.Second,
	}))

	//Setup Tracer
	if deps.OTTracer != nil {
		opts = append(opts,
			grpc.UnaryInterceptor(grpctrace.UnaryServerInterceptor(global.Tracer("SampleSvc-gRPC"))),
			grpc.StreamInterceptor(grpctrace.StreamServerInterceptor(global.Tracer("SampleSvc-gRPC"))),
		)
	}

	// setup Secure grpc connection,if requested
	if grpcCfg.Secure {
		// Load Server Certficate
		serverCert, err := tls.LoadX509KeyPair(grpcCfg.TLSCert, grpcCfg.TLSKey)
		if err != nil {
			deps.Fatal(ctx, "transport grpc secureStartup:", err)
		}

		// Setup Mutual Auth
		creds := credentials.NewTLS(&tls.Config{
			Certificates: []tls.Certificate{serverCert},
			ClientAuth:   tls.RequireAndVerifyClientCert,
		})
		opts = append(opts, grpc.Creds(creds))
	}

	baseServer = grpc.NewServer(opts...)
	pb.RegisterSampleServer(baseServer, grpcHndlr)

	grpcTrpt := GRPCTransport{
		Enabled: true,
		Start: func() error {
			deps.Infow(ctx, "Address", grpcCfg.Address(), "msg", "starting gRPC server ...")
			return baseServer.Serve(grpcListener)
		},
		Shutdown: func() {
			deps.Infow(ctx, "msg", "Shutting Down gRPC Server")
			baseServer.GracefulStop()
		},
	}
	return grpcTrpt, err
}

// ProvideDebugHndlr - Builds Debug HTTP Handler
func ProvideDebugHndlr(deps BaseDependencies) (dt DebugTransport, err error) {
	// The debug listener mounts the http.DefaultServeMux, and serves up
	// stuff like the Prometheus metrics route, the Go debug and profiling
	// routes, and so on.
	ctx := context.Background()
	addr := deps.Sample.Prometheus.Address()
	debugListener, err := net.Listen("tcp", addr)
	if err != nil {
		deps.Error(ctx, "main() - HTTP(debug) \t", err)
		return
	}
	dt = DebugTransport{
		Enabled: true,
		Start: func() error {
			deps.Infow(ctx, "Address", addr, "msg", "starting HTTP(debug) server ...")
			return http.Serve(debugListener, http.DefaultServeMux)
		},
		Shutdown: func() {
			deps.Infow(ctx, "msg", "Shutting down HTTP(debug) server ...")
			debugListener.Close()
		},
	}

	return dt, err
}

// ProvideSubsClient - Provide PubSub Client
func ProvideSubsClient(deps BaseDependencies, e endpoints.Endpoints) (s PubSubTransport, err error) {
	lg := log.New("PubSub", deps.Sample.Log)
	ctx := context.Background()
	subClnt := transport.NewSubClient(deps.Sample.PS.SubURL, lg, e) // Create Sub client to listen for events

	s = PubSubTransport{
		Enabled: true,
		Start: func() error {
			// Start PubSub Client
			if deps.Sample.PS.Enabled {
				lg.Debugw(ctx, "msg", "Listening on pubsub %s", deps.Sample.PS.SubURL)
				return subClnt.Receive()
			}
			return nil
		},
		Shutdown: func() {
			if subClnt.Subscription != nil {
				lg.Debugw(ctx, "Closing pubsub")
				subClnt.Subscription.Shutdown(context.Background())
			}
		},
	}
	return s, err
}

// ProvideInterruptHandlers - Sets up Interrupt Handlers.
func ProvideInterruptHandlers() (i InterruptHandler, err error) {
	// This function just sits and waits for ctrl-C.
	cancelInterrupt := make(chan struct{})
	return InterruptHandler{
		Enabled: true,
		Start: func() error {
			c := make(chan os.Signal, 1)
			signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
			select {
			case sig := <-c:
				err := fmt.Errorf("Quitting Service .. received signal %s", sig)
				return err
			case <-cancelInterrupt:
				return nil
			}
		},
		Shutdown: func() {
			close(cancelInterrupt)
			pprof.StopCPUProfile() // If enabled will stop cpu profile
		},
	}, nil
}

// HandleProfilers - Handle CPU and Mem Profiling requests
func HandleProfilers() (err error) {

	var (
		fs               = flag.NewFlagSet("Sample Service", flag.ExitOnError)
		ctx              = context.Background()
		cpuprofile       = fs.String("cpuprofile", "", "Write CPU Profile to `file`")
		memprofile       = fs.String("memprofile", "", "Write Mem Profile to `file`")
		cpufile, memfile *os.File
		log              = log.New("Profilers", config.LogCfg{Level: log.DebugLevel})
	)
	fs.Parse(os.Args[1:])

	// Start cpu profile if requested
	if *cpuprofile != "" {
		cpufile, err = os.Create(*cpuprofile)
		if err != nil {
			log.Errorw(ctx, "could not create CPU profile: ", err)
		}
		if err := pprof.StartCPUProfile(cpufile); err != nil {
			log.Errorw(ctx, "could not start CPU profile: ", err)
		}
		log.Debug(ctx, "Cpu Profiling Started")
	}
	// Start goroutine to capture memory stats, if requested
	// Add memprofile around functions need to be traced.. memprofile added here will dump stats every second
	go func() {
		if *memprofile != "" {
			for {
				time.Sleep(time.Second) //TODO change timing for memprofile, to get from config
				if *memprofile != "" {
					fmt.Println("Doing memprofile")
					memfile, err = os.Create(*memprofile)
					if err != nil {
						fmt.Println("could not create memory profile: ", err)
					}
					//runtime.GC() // get up-to-date statistics
					if err := pprof.WriteHeapProfile(memfile); err != nil {
						fmt.Println("could not write memory profile: ", err)
					}
				}
			}
		}
	}()
	// defer cpufile.Close()
	// defer memfile.Close()
	return err
}

// ProvideFunctionsList - Helper function to list up the supported functionalities
func ProvideFunctionsList(fn Functions) (fl FunctionsList) {

	for _, f := range []SampleFunction{
		SampleFunction(fn.HTTPTransport),
		SampleFunction(fn.GRPCTransport),
		SampleFunction(fn.DebugTransport),
		SampleFunction(fn.PubSubTransport),
		SampleFunction(fn.InterruptHandler),
	} {
		if f.Enabled {
			fl = append(fl, f)
		}
	}
	return fl
}
