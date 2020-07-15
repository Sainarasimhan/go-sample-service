package service

import (
	"context"
	"fmt"
	"strconv"

	"cloud.google.com/go/errorreporting"
	log "github.com/Sainarasimhan/Logger"
	svcerr "github.com/Sainarasimhan/go-error/err"
	repo "github.com/Sainarasimhan/sample/pkg/repository"
	"go.opentelemetry.io/otel/api/metric"

	"github.com/Sainarasimhan/sample/pb"
	Repo "github.com/Sainarasimhan/sample/pkg/repository"
)

//Service - interface defines the core service
type Service interface {
	Create(context.Context, CreateRequest) error
	CreateAsync(context.Context, CreateRequest) error
	List(context.Context, ListRequest) (Details, error)
}

// Publisher - interface to send domain events
type Publisher interface {
	Publish(context.Context, Event) error
}

//ListRequest - Request for List
type ListRequest struct {
	*pb.ListRequest
}

//Details - Response for List
type Details struct {
	*pb.Details
}

//CreateRequest  - Request for Create
type CreateRequest struct {
	*pb.CreateRequest
}

// Event - Structure to send event update
type Event struct {
	Msg    string
	Param1 string
}

// type implementing Service and holds all data for performing business logic
type sampleService struct {
	Repo.Repository
	Publisher
	*log.Logger
	er                 *errorreporting.Client
	conWrkrs           int
	createCh           chan (CreateRequest)
	createCnt, listCnt metric.BoundInt64Counter
}

// Option - service configuration options
type Option func(*sampleService)

// New - Creates new Service object
func New(lg *log.Logger, repo Repo.Repository, opt ...Option) (Service, error) {
	s := sampleService{
		Logger:     lg,
		Repository: repo,
		conWrkrs:   1,
	}

	for _, op := range opt {
		op(&s)
	}

	/* Create Async Workers for Create operation */
	s.createCh = make(chan (CreateRequest), s.conWrkrs)
	for i := 0; i < s.conWrkrs; i++ {
		go s.crWorkers()
	}

	//Setup Middleware
	svc := ErrorMiddleware(lg, s.er)(&s)
	svc = InstrumentingMiddleware(s.createCnt, s.listCnt)(svc)

	s.Info("Action", "NewService")("created new service type")
	return svc, nil
}

//SetAsyncWorkers - Option to set number of Async workers
func SetAsyncWorkers(concurrent int) Option {
	return func(s *sampleService) {
		s.conWrkrs = concurrent
	}
}

//SetCounters - Sets the open telemtry instruments in service layer
func SetCounters(reqCreate, reqList metric.BoundInt64Counter) Option {
	return func(s *sampleService) {
		s.createCnt, s.listCnt = reqCreate, reqList
	}
}

//SetPublishEvent - sets svc to publish events
func SetPublishEvent(p Publisher) Option {
	return func(s *sampleService) {
		s.Publisher = p
	}
}

//SetErrorReportingClient - sets client to do GCP error reporting
func SetErrorReportingClient(e *errorreporting.Client) Option {
	return func(s *sampleService) {
		s.er = e
	}
}

/* Interface Implementation */
// Create - Function creates new entries
func (s *sampleService) Create(ctx context.Context, cr CreateRequest) error {
	rReq := cr.repoRequest()

	_, err := s.Insert(ctx, rReq)
	if err != nil {
		s.Error("req", cr.String())("Error in Creating entry - %s", err.Error())
		return err
	}
	s.Info("req", cr.String())("Successfuly Created entry")

	//Publish Create Event
	if s.Publisher != nil {
		s.Publisher.Publish(ctx, cr.publishMsg())
	}
	return nil

}

// CreateAsync - Entries are created in async way
func (s *sampleService) CreateAsync(ctx context.Context, cr CreateRequest) error {
	select {
	case s.createCh <- cr:
		s.Info("req", cr.String())("Request pusehd to Async Queue")
	case <-ctx.Done():
		s.Error("req", cr.String())("Context Timeout, canceling request")
		return svcerr.DeadlineExceeded("Request Deadline Exceeded")
	}
	return nil
}

// Async workers to create entries
func (s *sampleService) crWorkers() {
	for cr := range s.createCh {
		s.Debug("req", cr.String())("Received Request from Async channel")
		if err := s.Create(context.Background(), cr); err != nil {
			// Report error happened during async creation
			if s.er != nil {
				s.er.Report(errorreporting.Entry{Error: err})
			}
		}
	}
}

// List - Gets the entries matching ID's
func (s *sampleService) List(ctx context.Context, lr ListRequest) (resp Details, err error) {
	var (
		rReq = lr.repoRequest()
		list = pb.Details{}
	)

	dtls, err := s.Repository.List(ctx, rReq)
	if err != nil {
		s.Error("req", lr.String())("Error in Getting entry - %s", err.Error())
		return
	}
	s.Info("req", lr.String(), "No_entries", strconv.Itoa(len(dtls)))("Got Details")
	for _, d := range dtls {
		entry := &pb.Detail{
			ID:     int32(d.ID),
			Param1: d.Param1,
			Param2: d.Param2,
			Param3: d.Param3,
		}
		list.Dtl = append(list.Dtl, entry)
	}
	resp.Details = &list
	return
}

/* Helper Funcs */
func (cr *CreateRequest) repoRequest() repo.Request {

	return Repo.Request{
		ID:     int(cr.GetID()),
		Param1: cr.GetParam1(),
		Param2: cr.GetParam2(),
		Param3: cr.GetParam3(),
	}
}

func (cr *CreateRequest) String() string {
	return fmt.Sprintf("ID=%d,TID=%s", cr.GetID(), cr.GetTransID())

}

func (cr *CreateRequest) publishMsg() Event {
	return Event{
		Msg:    "New Entry Created", //Pass ID & relavent info
		Param1: cr.Param1,
	}
}

func (lr *ListRequest) repoRequest() repo.Request {
	return Repo.Request{
		ID: int(lr.GetID()),
	}
}

func (lr *ListRequest) String() string {
	return fmt.Sprintf("ID=%d", lr.GetID())
}
