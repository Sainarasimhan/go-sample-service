package repo

import (
	"context"
	"fmt"

	log "github.com/Sainarasimhan/Logger"
	svcerr "github.com/Sainarasimhan/go-error/err"
	"github.com/lib/pq"
	"go.opentelemetry.io/otel/api/trace"
)

type commonMiddlware struct {
	Repository
	*log.Logger
	ot trace.Tracer
}

//MiddleWare - returns new implementation covering existing one
type MiddleWare func(Repository) Repository

//CommonMiddleware -  Does below functionalities
// - convert any DB error to standard error
// - Sets up Opentelemetry tracer for DB calls
func CommonMiddleware(lg *log.Logger, ot trace.Tracer) MiddleWare {
	return func(next Repository) Repository {
		lg.Debug()("Wrapped Repo with ErrorMiddelware")
		return &commonMiddlware{Repository: next, Logger: lg, ot: ot}
	}
}

func (cm *commonMiddlware) Insert(ctx context.Context, req Request) (Response, error) {
	if sp := cm.traceDB(ctx); sp != nil {
		defer sp.End()
	}
	resp, err := cm.Repository.Insert(ctx, req)
	if err != nil {
		return resp, cm.mapPostgresError(fmt.Sprintf("Insert with req %v", req), err)
	}
	return resp, err
}

func (cm *commonMiddlware) List(ctx context.Context, req Request) ([]Details, error) {
	if sp := cm.traceDB(ctx); sp != nil {
		defer sp.End()
	}
	dts, err := cm.Repository.List(ctx, req)
	if err != nil {
		return dts, cm.mapPostgresError(fmt.Sprintf("List with req %v", req), err)
	}
	if len(dts) == 0 {
		return dts, svcerr.NotFound(fmt.Sprintf("Entries not Found for ID %d", req.ID))
	}
	return dts, err
}

//Not Used atm
/*func (e *errorMiddleware) Delete(ctx context.Context, req Request) error {
	err := e.Repository.Delete(ctx, req)
	if err != nil {
		return e.mapPostgresError(fmt.Sprintf("Delete with req %v", req), err)
	}
	return err
}*/

func (cm *commonMiddlware) mapPostgresError(msg string, err error) (serr error) {
	if perr, ok := err.(*pq.Error); ok {
		cm.Debug("error", perr.Error())(fmt.Sprintf("Detailed error %#v", perr))
		detail := svcerr.DebugInfo{
			Detail: msg + "Detail:" + perr.Message,
		}
		switch perr.Code.Class() {
		case "2":
			return svcerr.NotFound(perr.Error(), &detail)
		case "23", "42":
			return svcerr.InvalidArgs(perr.Error(), &detail)
		case "53", "54":
			return svcerr.ResourceExhausted(perr.Error(), &detail)
		default:
			return svcerr.InternalErr(perr.Error(), &detail)
		}
	}
	return err
}

/*Func to start tracer for DB2 call*/
func (cm *commonMiddlware) traceDB(ctx context.Context) trace.Span {
	if cm.ot == nil {
		return nil
	}
	if span := trace.SpanFromContext(ctx); span != nil {
		_, sp := cm.ot.Start(ctx, "Postgres Database Call")
		return sp
	}
	_, sp := cm.ot.Start(ctx, "Asynchronous Postgres Database Call")
	return sp
}
