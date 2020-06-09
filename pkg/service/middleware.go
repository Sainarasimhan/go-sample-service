package service

import (
	"context"
	"fmt"

	log "github.com/Sainarasimhan/Logger"
	svcerr "github.com/Sainarasimhan/go-error/err"
	"go.opentelemetry.io/otel/api/metric"
)

type errorMiddleware struct {
	next Service
	*log.Logger
}

type instrumentingMiddleware struct {
	next               Service
	createCnt, listCnt metric.BoundInt64Counter
}

type middleware func(Service) Service

//ErrorMiddleware - Middleware to return uniform error types based of rpc codes.
func ErrorMiddleware(lg *log.Logger) middleware {
	return func(next Service) Service {
		return &errorMiddleware{next: next, Logger: lg}
	}
}

//InstrumentingMiddleware - Captures the number of requests received
func InstrumentingMiddleware(cr, ls metric.BoundInt64Counter) middleware {
	return func(next Service) Service {
		return &instrumentingMiddleware{next: next, createCnt: cr, listCnt: ls}
	}
}

func (e *errorMiddleware) Create(ctx context.Context, cr CreateRequest) (err error) {
	defer e.recoverFunc(&err)
	err = e.next.Create(ctx, cr)
	if err != nil {
		if !svcerr.IsValid(err) {
			err = svcerr.InternalErr(err.Error())
		}
	}
	return err
}

func (e *errorMiddleware) CreateAsync(ctx context.Context, cr CreateRequest) (err error) {
	defer e.recoverFunc(&err)
	err = e.next.CreateAsync(ctx, cr)
	if err != nil {
		if !svcerr.IsValid(err) {
			err = svcerr.InternalErr(err.Error())
		}
	}
	return err
}

func (e *errorMiddleware) List(ctx context.Context, lr ListRequest) (D Details, err error) {
	defer e.recoverFunc(&err)
	D, err = e.next.List(ctx, lr)
	if err != nil {
		if !svcerr.IsValid(err) {
			err = svcerr.InternalErr(err.Error())
		}
	}
	return D, err

}

func (e *errorMiddleware) recoverFunc(err *error) {
	if msg := recover(); msg != nil {
		e.Error("Panic", "handler")("panic received %v", msg)
		*err = svcerr.InternalErr(fmt.Sprintf("Panic handling request, error - %s", msg))
	}
}

func (i *instrumentingMiddleware) Create(ctx context.Context, cr CreateRequest) (err error) {
	err = i.next.Create(ctx, cr)
	i.createCnt.Add(ctx, 1)
	return
}

func (i *instrumentingMiddleware) CreateAsync(ctx context.Context, cr CreateRequest) (err error) {
	err = i.next.CreateAsync(ctx, cr)
	i.createCnt.Add(ctx, 1)
	return
}

func (i *instrumentingMiddleware) List(ctx context.Context, lr ListRequest) (D Details, err error) {
	D, err = i.next.List(ctx, lr)
	i.listCnt.Add(ctx, 1)
	return
}
