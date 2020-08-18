package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"

	"github.com/Sainarasimhan/sample/pb"

	"google.golang.org/grpc"
)

func main() {

	f := flag.NewFlagSet("gRPC client", flag.ExitOnError)
	var (
		num = f.Int("n", 100, "Number of requests")
		con = f.Int("c", 10, "Concurrent Requests")
	)
	f.Parse(os.Args[1:])
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

	for j := 0; j < *num / *con; j++ {
		st := stats{}
		for i := 0; i < *con; i++ { //MAX Parallel Calls
			go func() {
				defer func(t time.Time) {
					dur := time.Since(t)
					//	log.Println("Time taken", dur)
					if st.min == 0 || dur < st.min {
						st.min = dur
					} else if dur > st.max {
						st.max = dur
					}
					st.ttime += dur
				}(time.Now())
				//Get List
				_, err := client.List(context.Background(), &lr)
				if err != nil {
					//log.Printf("Got Error from List Request %s\n", err)
					st.fail++
				} else {
					//log.Println("response", resp)
					st.succ++
				}
				st.total++

				//Get List Stream
				/*stream, err := client.ListStream(context.Background(), &lr)
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
				} */
			}()
		}
		<-time.After(5 * time.Second) //Give some time for calls to finish
		// Print stats
		log.Println("-----------------------")
		log.Println("Total Calls:", st.total)
		log.Println("Success Calls:", st.succ)
		log.Println("Failure Calls:", st.fail)
		log.Println("Min Duration:", st.min)
		log.Println("Max Duration:", st.max)
		log.Println("Avg Duration:", st.ttime/time.Duration(st.total))
		log.Println("-----------------------")

	}

}

type stats struct {
	succ  int
	fail  int
	total int
	min   time.Duration
	max   time.Duration
	ttime time.Duration
}
