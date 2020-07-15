package transport

import (
	"context"
	"errors"
	"strconv"

	"cloud.google.com/go/pubsub"
	log "github.com/Sainarasimhan/Logger"
	"github.com/Sainarasimhan/sample/pb"
	"github.com/Sainarasimhan/sample/pkg/endpoints"
	"github.com/Sainarasimhan/sample/pkg/service"
	"github.com/go-kit/kit/endpoint"
)

// Support pubsub as a transport
// Listen for Msgs and invoke endpoints with proper request
// Specific to GCP - pub/sub

type PubSubClient interface {
	Receive() error //
	Close()
}

type pubsubCallBack func(context.Context, *pubsub.Message)
type DecodeReqFunc func(context.Context, *log.Logger, *pubsub.Message) (interface{}, error)

type psClient struct {
	*pubsub.Client
	*pubsub.Subscription
	*pubsub.Topic
	handler pubsubCallBack
	*log.Logger
	//timeout
}

type PubSubCfg struct {
	ProjectID    string
	Topic        string
	Subscription string
}

//NewSub - creates pubsub client
func NewSub(cfg PubSubCfg, lg *log.Logger, e endpoints.Endpoints) PubSubClient {
	client, err := pubsub.NewClient(context.Background(), cfg.ProjectID)
	if err != nil {
		lg.Println(err)
		return nil
	}

	//create instance of pubsub type
	pc := psClient{Client: client,
		Logger: lg}

	// Create Subscription
	if cfg.Subscription != "" {
		pc.Subscription = client.Subscription(cfg.Subscription)
		// form Handler
		pc.createPubSubHndlr(decodePubSubReq, e.Create)
	}

	return &pc
}

func NewPublishClient(lg *log.Logger, cfg PubSubCfg) service.Publisher {

	client, err := pubsub.NewClient(context.Background(), cfg.ProjectID)
	if err != nil {
		lg.Println(err)
		return nil
	}

	//create instance of pubsub type
	pc := psClient{Client: client,
		Logger: lg}

	// Create Subscription
	if cfg.Topic != "" {
		pc.Topic = client.Topic(cfg.Topic)
	}

	fn := func(ctx context.Context, request interface{}) (response interface{}, err error) {

		msg, _ := request.(*pubsub.Message)
		res := pc.Topic.Publish(ctx, msg)

		// Handle response async - log any error
		go func(res *pubsub.PublishResult) {
			msgID, err := res.Get(ctx)
			if err != nil {
				lg.Error()(err.Error())
			} else {
				lg.Info("PubSub", "Publish")("Message Sent %v \n", msgID)
			}

		}(res)
		return nil, nil

	}
	//create instance of pubsub type
	return &endpoints.Endpoints{Pub: fn}
}

func (p *psClient) Receive() error {
	if p.Subscription == nil {
		return errors.New("Invalid Subscription")
	}
	err := p.Subscription.Receive(context.Background(), p.handler)
	if err != nil {
		p.Println(err)
	}
	return err
}

func (p *psClient) Close() {
	p.Subscription.Delete(context.Background())
	p.Client.Close()
}

func decodePubSubReq(ctx context.Context, lg *log.Logger, m *pubsub.Message) (request interface{}, err error) {
	// Convert pub sub message to protobuf message
	var (
		msg   = &pb.CreateRequest{}
		epReq = endpoints.CreateRequest{}
	)
	id, _ := strconv.Atoi(m.ID)
	msg.ID = int32(id)

	msg.Param1 = m.Attributes["Param1"]
	msg.Param2 = m.Attributes["Param2"]
	msg.Param3 = m.Attributes["Param3"]
	msg.TransID = m.Attributes["TransID"]

	// validate request
	if err = ValidateRequest(msg); err != nil {
		lg.Println(err)
		return msg, err
	}

	// return endpoint request struct
	epReq.Cr.CreateRequest = msg
	return epReq, nil
}

func (c *psClient) createPubSubHndlr(d DecodeReqFunc, e endpoint.Endpoint) {
	c.handler = func(ctx context.Context, m *pubsub.Message) {

		// Decode message
		r, err := d(ctx, c.Logger, m)
		if err != nil {
			// Log error on receiving wrong event
			m.Ack()
			return // Just log or return
		}

		_, err = e(ctx, r)
		if err != nil {
			// if not processed, message will be picked up again
			return // log error
		}

		//Acknowledge message
		m.Ack()
	}
}
