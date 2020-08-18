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

// TODO
// Add Testing for individual packages
// More Documentation
// Improve Error Handling for dependencies

package main

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
	"text/tabwriter"
	"time"

	"github.com/Sainarasimhan/sample/pb"
	config "github.com/Sainarasimhan/sample/pkg/cfg"
	"github.com/Sainarasimhan/sample/pkg/endpoints"
	repo "github.com/Sainarasimhan/sample/pkg/repository"
	"github.com/Sainarasimhan/sample/pkg/service"
	transport "github.com/Sainarasimhan/sample/pkg/transport"

	newlog "github.com/Sainarasimhan/sample/pkg/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"

	"github.com/oklog/oklog/pkg/group"
	"go.opentelemetry.io/otel/api/global"
	"go.opentelemetry.io/otel/api/kv"
	"go.opentelemetry.io/otel/api/trace"
	metricstdout "go.opentelemetry.io/otel/exporters/metric/stdout"
	"go.opentelemetry.io/otel/plugin/grpctrace"
	"go.opentelemetry.io/otel/sdk/metric/controller/push"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"go.opentelemetry.io/otel/api/metric"
	"go.opentelemetry.io/otel/exporters/trace/stdout"

	//Debug info
	_ "net/http/pprof"
	//Export Debug vars
	_ "expvar"
)

//go:generate protoc --go_out=plugins=grpc:. -I ../ pb/sample.proto

func main() {

	// Flags allow interaction with the command line arguments
	fs := flag.NewFlagSet("sample", flag.ExitOnError)
	var (
		debugAddr  = fs.String("debug.addr", ":8080", "Debug and metrics listen address")
		httpAddr   = fs.String("http-addr", ":8081", "HTTP listen address")
		grpcAddr   = fs.String("grpc-addr", ":8082", "gRPC listen address")
		zipkinURL  = fs.String("zipkin-url", "", "Enable Zipkin tracing via HTTP reporter URL e.g. http://localhost:9412/api/v2/spans")
		cpuprofile = fs.String("cpuprofile", "", "write cpu profile to `file`")
		memprofile = fs.String("memprofile", "", "write memory profile to `file`")
		lg         = newlog.NewZapLogger("Main", config.LogCfg{Level: newlog.DebugLevel})
		//lg         = log.New(log.Cfg{Level: log.LevelDebug, Prefix: "SampleSvc ", Flags: log.LstdFlags}) // Create Loggers for diff Layers
		//repolg     = log.New(log.Cfg{Level: log.LevelDebug, Prefix: "Repository", Flags: log.LstdFlags})
		err error
		ctx = context.Background() // Base ctx used to startup
	)
	fs.Usage = usageFor(fs, os.Args[0]+" [flags]")
	fs.Parse(os.Args[1:])

	// Update From cfg file
	var cfg *config.Cfg
	{
		cfg, err = config.Init()
		if err != nil {
			lg.Fatal(ctx, "Config error", err) // Service will exit on any config errors
		} else {
			c := cfg.Sample
			for addr, val := range map[*string]string{ // map the address from config file to vars
				httpAddr:  c.HTTP.Address(),
				grpcAddr:  c.GRPC.Address(),
				zipkinURL: c.Zipkin.Address(),
				debugAddr: c.Prometheus.Address(),
			} {
				if val != "" && addr != nil {
					*addr = val
				}
			}
		}
	}

	// Setup Tracer
	var otTracer trace.Tracer
	{
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
			lg.Fatal(ctx, err)
		}

		tp, err := sdktrace.NewProvider(sdktrace.WithConfig(sdktrace.Config{DefaultSampler: sdktrace.ProbabilitySampler(.1)}),
			sdktrace.WithSyncer(exporter))
		if err != nil {
			lg.Fatal(ctx, err)
		}

		global.SetTraceProvider(tp)
		otTracer = global.Tracer("SampleSvc")
	}

	// Setup Metrics
	{
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
			lg.Fatal(ctx, "failed to initialize metric stdout exporter %v", err)
		}
		defer pusher.Stop()

	}

	// Create Metric Instruments
	var reqCreateSvc, reqListSvc metric.BoundInt64Counter
	{
		if reqCreateCnt, err := global.Meter("sample").NewInt64Counter("Sample_Svc Create_Requests"); err != nil {
			lg.Error(ctx, "Action", "MetricsCreation", "Error creating Metrics %s", err.Error())
		} else {
			reqCreateSvc = reqCreateCnt.Bind(kv.String("request", "createsvc"))
			defer reqCreateSvc.Unbind()
		}
		if reqtListCnt, err := global.Meter("sample").NewInt64Counter("Sample_Svc List_Requests"); err != nil {
			lg.Error(ctx, "Action", "MetricsCreation", "Error creating Metrics %s", err.Error())
		} else {
			reqListSvc = reqtListCnt.Bind(kv.String("request", "Listsvc"))
			defer reqListSvc.Unbind()
		}
	}

	var (
		reqLatency metric.Float64ValueRecorder
		dbMetrics  repo.Metrics
	)
	{
		{
			if reqLatency, err = global.Meter("sample").NewFloat64ValueRecorder("Request Latency"); err != nil {
				lg.Error(ctx, "Action", "MetricsCreation", "Error creating Metrics %s", err.Error())
			}
		}

		batchObserver := global.Meter("Sampple-DB").NewBatchObserver(dbMetrics.DoMetrics())
		dbMetrics.OpenCnx, _ = batchObserver.NewInt64ValueObserver("Opne DB Connections")
		dbMetrics.IdleCnx, _ = batchObserver.NewInt64ValueObserver("Idle DB Connections")
		dbMetrics.IdelClosed, _ = batchObserver.NewInt64SumObserver("Idle Close DB Connections")
	}

	// Create Main components of service,
	// Create repo - db backend
	var db repo.Repository
	{
		repolg := newlog.NewZapLogger("Repo", cfg.DB.Log)        // Logger for Repo Layer
		db, err = repo.NewPostgres(cfg.DB.PostgresStr(), repolg, // Postgres
			repo.SetMaxPostgresConn(cfg.DB.MaxConns),
			repo.SetTracer(otTracer),
			repo.EnableMetrics(&dbMetrics))
		if err != nil {
			lg.Fatal(ctx, "Error creating repo", err) // Service exits on any error creating repo
		} else {
			defer db.Close()
		}
	}

	// Create Service, Endpoints and Transport Handlers
	// service created is passed to endpoints,
	// endpoints is passed to transport handlers, along with other dependencies.
	svc, _ := service.New(
		newlog.New("Svc", cfg.Sample.Log),
		db,
		// Service Options
		service.SetAsyncWorkers(5),
		service.SetCounters(reqCreateSvc, reqListSvc),
		/* Publish Client and Error reporting client can be setup as below.
		service.SetPublishEvent(
		transport.NewPubClient(lg, cfg.Sample.PS.PubURL)), //Create Publish Event to push notifications
		service.SetErrorReportingClient(
			service.NewErrorReportingClient(lg, cfg.Sample.ErrRprtPrjID)), //Setup GCP Error reporting client
		*/
	)

	endpoint := endpoints.New(svc, newlog.New("EndPoint", cfg.Sample.Log), reqLatency)
	httpHndlr := http.TimeoutHandler(
		transport.NewHTTPHandler(endpoint, otTracer),
		transport.Timeout,
		transport.TimeoutError)
	grpcHndlr := transport.NewGRPCServer(endpoint)
	subClnt := transport.NewSubClient(cfg.Sample.PS.SubURL, lg, endpoint) // Create Sub client to listen for events

	// Now we're to the part of the func main where we want to start actually
	// running things, like servers bound to listeners to receive connections.
	//
	// The method is the same for each component: add a new actor to the group
	// struct, which is a combination of 2 anonymous functions: the first
	// function actually runs the component, and the second function should
	// interrupt the first function and cause it to return. It's in these
	// functions that we actually bind the Go kit server/handler structs to the
	// concrete transports and run them.
	//
	// Putting each component into its own block is mostly for aesthetics: it
	// clearly demarcates the scope in which each listener/socket may be used.

	var g group.Group
	{
		// The debug listener mounts the http.DefaultServeMux, and serves up
		// stuff like the Prometheus metrics route, the Go debug and profiling
		// routes, and so on.
		debugListener, err := net.Listen("tcp", *debugAddr)
		if err != nil {
			lg.Fatal(ctx, "main() - HTTP(debug) \t", err)
		}
		g.Add(func() error {
			lg.Infow(ctx, "Address", *debugAddr, "msg", "starting HTTP(debug) server ...")
			return http.Serve(debugListener, http.DefaultServeMux)
		}, func(error) {
			debugListener.Close()
		})
	}
	{
		// The HTTP listener mounts the Go kit HTTP handler we created.
		var (
			httpCfg           = cfg.Sample.HTTP
			srv               *http.Server
			httpListener, err = net.Listen("tcp", *httpAddr)
		)
		if err != nil {
			lg.Fatal(ctx, "main() - HTTP \t ", err)
		}
		g.Add(func() error {
			lg.Infow(ctx, "Address", *httpAddr, "msg", "starting HTTP server ...")
			srv = &http.Server{ // Setup server with Timeouts
				Handler:      httpHndlr,
				ReadTimeout:  10 * time.Second,
				WriteTimeout: 10 * time.Second,
				TLSConfig: &tls.Config{
					ClientAuth: tls.RequireAndVerifyClientCert,
				},
			}
			if httpCfg.Secure {
				return srv.ServeTLS(httpListener, httpCfg.TLSCert, httpCfg.TLSKey)
			}
			return srv.Serve(httpListener)
		}, func(error) {
			sdctx, sdcancel := context.WithTimeout(context.Background(), 60*time.Second)
			// wait for 60 seconds max
			defer sdcancel()
			if srv != nil {
				srv.Shutdown(sdctx)
			}
		})
	}
	{
		// The gRPC listener mounts the Go kit gRPC server we created.
		var (
			grpcCfg           = cfg.Sample.GRPC
			grpcListener, err = net.Listen("tcp", *grpcAddr)
			baseServer        *grpc.Server
		)
		if err != nil {
			lg.Fatal(ctx, "transport gRPC \t", err)
		}
		g.Add(func() error {
			lg.Infow(ctx, "Address", *grpcAddr, "msg", "starting gRPC server ...")
			opts := []grpc.ServerOption{}

			//Add KeepAlive Timeout
			opts = append(opts, grpc.KeepaliveParams(keepalive.ServerParameters{
				MaxConnectionAge:      5 * time.Minute,
				MaxConnectionAgeGrace: 5 * time.Second,
			}))

			//Setup Tracer
			if otTracer != nil {
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
					lg.Fatal(ctx, "transport grpc secureStartup:", err)
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
			return baseServer.Serve(grpcListener)
		}, func(error) {
			baseServer.GracefulStop()
		})
	}
	{
		// Start PubSub Client
		if cfg.Sample.PS.Enabled {
			g.Add(func() error {
				return subClnt.Receive()
			}, func(err error) {
				lg.Debugw(ctx, "Closing pubsub %s", err.Error())
				if subClnt.Subscription != nil {
					subClnt.Subscription.Shutdown(context.Background())
				}
			})
		}
	}
	{
		// This function just sits and waits for ctrl-C.
		cancelInterrupt := make(chan struct{})
		g.Add(func() error {
			c := make(chan os.Signal, 1)
			signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
			select {
			case sig := <-c:
				err := fmt.Errorf("Quitting Service .. received signal %s", sig)
				return err
			case <-cancelInterrupt:
				return nil
			}
		}, func(error) {
			close(cancelInterrupt)
		})
	}
	{
		// Start cpu profile if requested
		if *cpuprofile != "" {
			f, err := os.Create(*cpuprofile)
			if err != nil {
				lg.Fatal(ctx, "could not create CPU profile: ", err)
			}
			defer f.Close()
			if err := pprof.StartCPUProfile(f); err != nil {
				lg.Fatal(ctx, "could not start CPU profile: ", err)
			}
			lg.Debug(ctx, "Cpu Profiling Started")
			defer pprof.StopCPUProfile()
		}
	}
	{
		// Start goroutine to capture memory stats, if requested
		go func() {
			if *memprofile != "" {
				for {
					time.Sleep(time.Millisecond * 500) //TODO change timing for memprofile, to get from config
					memProfile(memprofile)
				}
			}
		}()
	}

	lg.Errorw(ctx, "exit", g.Run().Error())
}

func usageFor(fs *flag.FlagSet, short string) func() {
	return func() {
		fmt.Fprintf(os.Stderr, "USAGE\n")
		fmt.Fprintf(os.Stderr, "  %s\n", short)
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "FLAGS\n")
		w := tabwriter.NewWriter(os.Stderr, 0, 2, 2, ' ', 0)
		fs.VisitAll(func(f *flag.Flag) {
			fmt.Fprintf(w, "\t-%s %s\t%s\n", f.Name, f.DefValue, f.Usage)
		})
		w.Flush()
		fmt.Fprintf(os.Stderr, "\n")
	}
}

func memProfile(memprofile *string) {
	if *memprofile != "" {
		fmt.Println("Doing memprofile")
		f, err := os.Create(*memprofile)
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
