package endpoints

import (
	"context"
	"fmt"
	"time"

	log "github.com/Sainarasimhan/sample/pkg/log"
	"github.com/go-kit/kit/endpoint"
	"go.opentelemetry.io/otel/api/kv"
	"go.opentelemetry.io/otel/api/metric"
)

/*middlware to perfrom below functionality
- Logging
- Instrumentation
*/

//LoggingMiddleware  - Logs the request/response details
func LoggingMiddleware(lg log.Logger) endpoint.Middleware {
	return func(e endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, request interface{}) (response interface{}, err error) {
			defer func(begin time.Time) {
				//TODO print only allowed fields
				lg.Infow(ctx, "err", err, "Time Taken", time.Since(begin))
			}(time.Now())
			response, err = e(ctx, request)
			return
		}
	}
}

//InstrumentingMiddelware - Captures time consumed per request.
//Open Telemetry value Recorder used to capture the request processing time
func InstrumentingMiddelware(duration metric.Float64ValueRecorder) endpoint.Middleware {
	return func(e endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, request interface{}) (response interface{}, err error) {
			defer func(begin time.Time) {
				//duration.With("method", "endpoint", "Success", fmt.Sprint(err == nil)).Observe(time.Since(begin).Seconds())
				duration.Record(ctx, time.Since(begin).Seconds(), kv.String("Success", fmt.Sprint(err == nil)))
			}(time.Now())
			response, err = e(ctx, request)
			return
		}
	}
}
