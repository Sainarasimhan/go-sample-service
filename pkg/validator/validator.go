package validator

//Validator for protobuf messages.

//Supported validators
// -- required
// -- string  - min length, max length, allowed list
// -- int  - min value, max value

// Supported types
// -- Messages, string, integers
// Support for other types and lists,enums,structs to be added.

// Validator returns errors as Request Field Violations (google-api's errdetails)

import (
	"errors"
	"fmt"

	valid "github.com/Sainarasimhan/sample/pb"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

type violation = errdetails.BadRequest_FieldViolation
type violations = []*errdetails.BadRequest_FieldViolation

type validator struct {
	vlist violations
	max   int
}

//GetViolations - Get Request violiations from proto Message
func GetViolations(msg protoreflect.Message, max int) violations {

	var (
		vld    = validator{max: max}
		err    error
		fields = msg.Descriptor().Fields()
	)

	for i := 0; i < fields.Len(); i++ {
		desc := fields.Get(i)
		values := msg.Get(desc)
		opts := desc.Options().(*descriptorpb.FieldOptions)
		req := proto.GetExtension(opts, valid.E_Required).(bool)

		switch desc.Kind() {
		case protoreflect.MessageKind:
			if req {
				if desc.IsList() {
					iv := values.List()
					for i := 0; i < iv.Len(); i++ {
						va := iv.Get(i)
						if err = vld.verifyMessage(va); err != nil {
							return vld.vlist
						}
					}
				} else {
					if err = vld.verifyMessage(values); err != nil {
						return vld.vlist
					}
				}
			}

		case protoreflect.StringKind:
			var vptr *violation

			allowedList := proto.GetExtension(opts, valid.E_Allowed).(*valid.StringList)
			if req && (len(values.String()) == 0) {
				vptr = mandatoryViolation(desc.FullName())
			}

			//Validate Allowed string
			if allowedList != nil && len(values.String()) != 0 {
				if searchSlice(allowedList.List, values.String()) == false {
					//Value Not allowed
					vptr = notAllowedViolation(desc.Name(), values.String())
				}
			} else if len(values.String()) != 0 {

				min := proto.GetExtension(opts, valid.E_Lmin).(int32)
				max := proto.GetExtension(opts, valid.E_Lmax).(int32)

				if (min != 0 && len(values.String()) < int(min)) ||
					(max != 0 && len(values.String()) > int(max)) {

					vptr = &violation{
						Field: fmt.Sprintf("%s", desc.Name()),
						Description: fmt.Sprintf("Invalid Length, have (%d), want min(%d), max (%d)",
							len(values.String()), min, max),
					}
				}
			}

			if err = vld.appendViolations(vptr); err != nil {
				return vld.vlist
			}

		case protoreflect.Int32Kind, protoreflect.Int64Kind:
			{
				var vptr *violation
				if req && values.Int() == 0 {
					vptr = mandatoryViolation(desc.FullName())
				} else if values.Int() != 0 {
					min, max := proto.GetExtension(opts, valid.E_Min).(int32), proto.GetExtension(opts, valid.E_Max).(int32)
					if (min != 0 && values.Int() < int64(min)) || (max != 0 && values.Int() > int64(max)) {
						vptr = &violation{
							Field:       fmt.Sprintf("%s", desc.Name()),
							Description: fmt.Sprintf("Invalid Value, have (%d), want min(%d), max (%d)", values.Int(), min, max),
						}

					}
				}
				if err = vld.appendViolations(vptr); err != nil {
					return vld.vlist
				}
			}
		} // switch on descriptor kind
	} // for loop
	return vld.vlist
}

func (vld *validator) verifyMessage(v protoreflect.Value) error {
	var (
		err error
	)
	if v.Message().IsValid() {
		vt := GetViolations(v.Message(), 5)
		if vld.appendViolations(vt...); err != nil {
			return err
		}
	} else {
		v := mandatoryViolation(v.Message().Descriptor().FullName())
		if err = vld.appendViolations(v); err != nil {
			return err
		}
	}
	return nil
}

func (vld *validator) appendViolations(v ...*violation) error {
	var err error
	for _, val := range v {
		if val != nil {
			vld.vlist = append(vld.vlist, val)
			if len(vld.vlist) >= vld.max {
				err = errors.New("Max Violations reached")
			}
		}
	}
	return err
}

func mandatoryViolation(f protoreflect.FullName) *violation {
	v := violation{
		Field:       fmt.Sprintf("%s", f),
		Description: "mandatory field not provided",
	}
	return &v
}

func notAllowedViolation(n protoreflect.Name, val string) *violation {
	v := violation{
		Field:       fmt.Sprintf("%s", n),
		Description: fmt.Sprintf("Value (%s) not allowed", val),
	}
	return &v
}

func searchSlice(slice []string, s string) bool {
	for _, v := range slice {
		if s == v {
			return true
		}
	}
	return false
}
