package main

import (
	"context"
	"io"
	"log"
	"sample/pb"
	"time"

	"google.golang.org/grpc"
)

func main() {
	//Call grpc Server

	//Create gRPC conn
	conn, err := grpc.Dial(":8085", grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	//Create gRPC client
	client := pb.NewSampleClient(conn)

	//Create Request
	cr := pb.CreateRequest{
		ID:      1234,
		Param1:  "Test1",
		Param2:  "Test2",
		Param3:  "Test3",
		TransID: "Test-Random",
	}

	//Send Request
	_, err = client.Create(context.Background(), &cr)
	if err != nil {
		log.Printf("Got Error from Create Request %s\n", err)
	}

	//Send Async Request -- TODO
	_, err = client.Create(context.Background(), &cr)
	if err != nil {
		log.Printf("Got Error from Create Request %s\n", err)
	}

	//Create List Request
	lr := pb.ListRequest{
		ID:      1234,
		TransID: "Random",
	}

	for i := 0; i < 10; i++ { //MAX Parallel Calls
		go func() {
			log.Println("Start new Thread")
			//Get List
			resp, err := client.List(context.Background(), &lr)
			if err != nil {
				log.Printf("Got Error from Create Request %s\n", err)
			} else {
				log.Println("response", resp)
			}

			//Get List Stream
			stream, err := client.ListStream(context.Background(), &lr)
			if err != nil {
				log.Printf("Got Error from Create Request %s\n", err)
			} else {
				for {
					l, err := stream.Recv()
					if err == io.EOF {
						break
					} else if err != nil {
						log.Println(err)
						break
					} else {
						log.Println(l)
					}
				}
			}
		}()
	}

	<-time.After(5 * time.Second) //Give some time for calls to finish
}
