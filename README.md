# Sample Golang Micro-Servcie

  Built with go-kit and has 3 below components, 
  - transport - Supports REST and gRPC and does request validation. 
  - endpoints - Wraps Service Layer and provides non business logging,instrumentation along with 
			  rate limiting,circuit breaker functionality. 
  - service   - Implements Actual business logic. Support synchronous and asynchronous requests and 
			  does business specific instrumentation and logging. 

  Repository - Postgres Database is used as backend and all DB logic is abstracted in repo package. \
  Non-Functional logic are implemented as middelwares e.g. logging, error handling in each major component. 

  uses logging, error handling and config package from other git repos, along with few third party packages. 

## TODO
 - Add Testing for individual packages 
 - More Documentation 
 - Improve Error Handling for dependencies

## Notes 
 - Pub/Sub - Setup Google cloud account details to use pub/sub functionality
 - GCP Error Reporting - Setup GCP account to use error reporting
   - GOOGLE_APPLICATION_CREDENTIALS env var should be set with service account details
 - Protobuf generation - use make.sh to generate protobuf and grpc packages.