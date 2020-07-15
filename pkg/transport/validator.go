package transport

import (
	"errors"
	"fmt"

	"github.com/Sainarasimhan/sample/pb"
	"github.com/Sainarasimhan/sample/pkg/validator"

	svcerr "github.com/Sainarasimhan/go-error/err"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
)

//ValidateRequest - Validates Create/List requests as per protobuf definitions
func ValidateRequest(req interface{}) error {
	var preq protoreflect.Message
	switch t := req.(type) {
	// Valid Types
	case *pb.CreateRequest:
		preq = req.(*pb.CreateRequest).ProtoReflect()
	case *pb.ListRequest:
		preq = req.(*pb.ListRequest).ProtoReflect()
	default:
		errstr := fmt.Sprintf("Invalid Request type %s", t)
		return errors.New(errstr)
	}

	v := validator.GetViolations(preq, 5) // Max 5 Violations reported back
	if len(v) != 0 {
		Fv := svcerr.BadRequest{
			FieldViolations: v,
		}
		err := svcerr.InvalidArgs(
			"Request Validation Failure",
			&Fv,
		)
		return err
	}
	return nil
}
