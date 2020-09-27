FROM amd64/golang:1.15

RUN mkdir -p /src/sample-service
WORKDIR /src/sample-service

ADD ./ ./ 

RUN go mod verify
RUN go get github.com/google/wire/cmd/wire
#RUN go generate cmd/main.go

#ENV
ENV GOOS=linux 
ENV GOARCH=amd64 
ENV CGO_ENABLED=0

#vet, test, scan - TODO
RUN wire gen -output_file_prefix main- cmd/main.go
RUN go build -o SampleApp cmd/main-wire_gen.go 

#Run image
FROM alpine:latest
COPY --from=0 /src/sample-service/resources/ ./appl/resources/
COPY --from=0 /src/sample-service/SampleApp ./appl/

WORKDIR ./appl/
CMD ["./SampleApp"]