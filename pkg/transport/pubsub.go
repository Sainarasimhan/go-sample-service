package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/Sainarasimhan/sample/pb"
	"github.com/Sainarasimhan/sample/pkg/endpoints"
	log "github.com/Sainarasimhan/sample/pkg/log"
	"github.com/Sainarasimhan/sample/pkg/service"
	"github.com/go-kit/kit/endpoint"
	"gocloud.dev/pubsub"
	_ "gocloud.dev/pubsub/gcppubsub"
)

// Implementation with google cloud SDK Pub/Sub

type psClient struct {
	*pubsub.Subscription
	handler pubsubCallBack
	*pubsub.Topic
	log.Logger
}

type pubsubCallBack func(context.Context, *pubsub.Message)
type DecodeReqFunc func(context.Context, log.Logger, *pubsub.Message) (interface{}, error)

// NewSubClient returns gocloud subscription client
// TODO pass tracer
func NewSubClient(url string, lg log.Logger, e endpoints.Endpoints) *psClient {

	ctx := context.Background()
	subscription, err := pubsub.OpenSubscription(ctx,
		url)
	//"gcppubsub://projects/my-project/subscriptions/my-subscription")
	if err != nil {
		lg.Errorw(ctx, "PubSub", "Creation", "err", err)
	}

	// create local type
	ps := psClient{
		Subscription: subscription,
		Logger:       lg,
		handler:      createSubHndlr(decodeSubReq, lg, e.Create),
	}
	return &ps

}

func (s *psClient) Receive() (err error) {
	if s.Subscription == nil {
		return errors.New("Invalid Subscription")
	}

	for {
		ctx := context.Background() // TODO - Setup up context values
		msg, err := s.Subscription.Receive(ctx)
		if err != nil {
			s.Errorw(ctx, "PubSub", "Subscription", "err", err)
			return err
		}

		go func() {
			s.handler(ctx, msg)
		}()
	}
}

func (s *psClient) Shutdown() {
	if s.Subscription != nil {
		s.Subscription.Shutdown(context.Background())
	}
}

func decodeSubReq(ctx context.Context, lg log.Logger, m *pubsub.Message) (request interface{}, err error) {
	// Convert pub sub message to protobuf message
	var (
		msg   = &pb.CreateRequest{}
		epReq = endpoints.CreateRequest{}
	)

	// Do Json Decoding of msg
	json.NewDecoder(bytes.NewReader(m.Body)).Decode(msg)

	// validate request
	if err = ValidateRequest(msg); err != nil {
		lg.Error(ctx, err)
		return msg, err
	}

	// return endpoint request struct
	epReq.Cr.CreateRequest = msg
	return epReq, nil
}

func createSubHndlr(d DecodeReqFunc, lg log.Logger, e endpoint.Endpoint) pubsubCallBack {
	handler := func(ctx context.Context, m *pubsub.Message) {

		// Decode message
		r, err := d(ctx, lg, m)
		if err != nil {
			// Log error on receiving wrong event
			m.Ack()
			return // Just log or return
		}

		_, err = e(ctx, r)
		if err != nil {
			// if not processed, message will be picked up again
			if m.Nackable() {
				m.Nack()
			}
			return // log error
		}

		//Acknowledge message
		m.Ack()
	}
	return handler
}

// NewPubClient Publish Client
func NewPubClient(lg log.Logger, topicURL string) service.Publisher {

	ctx := context.Background()
	topic, err := pubsub.OpenTopic(ctx,
		topicURL)
	//"gcppubsub://projects/my-project/subscriptions/my-subscription")
	if err != nil {
		lg.Errorw(ctx, "Pub", "Creation", "err", err)
	}
	// Topic not closed .. if needed call topic.Shutdown()

	// create local type
	pc := psClient{
		Logger: lg,
		Topic:  topic,
	}

	//create instance of pubsub type
	return &endpoints.Endpoints{Pub: pc.Endpoint()}
}

//Endpoint - Creates endpoint from pubclient
func (pc *psClient) Endpoint() endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (response interface{}, err error) {

		msg, _ := request.(*pubsub.Message)
		err = pc.Topic.Send(ctx, msg)

		if err != nil {
			pc.Error(ctx, err)
		} else {
			pc.Infow(ctx, "PubSub", "Publish Message Sent")
		}

		return nil, err
	}
}
