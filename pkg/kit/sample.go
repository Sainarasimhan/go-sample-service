package service

import (
	"context"
)

//File to create go-kit scaffholdings
//TODO clean up and add correct structures

//SampleService - interface
type SampleService interface {
	Create(ctx context.Context, ir InsertRequest) error
	List(ctx context.Context, nr NotificationRequest) (Notification, error)
}

//NotificationRequest - Request
type NotificationRequest struct {
	ID int
}

//Notification -- user notifications
type Notification struct {
	ID          int
	Description string
	//	Timestamp   time.Time
}

//InsertRequest -- request
type InsertRequest struct {
	Params string
}
