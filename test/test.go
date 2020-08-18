// +build wireinject
// The build tag makes sure the stub is not built in the final build.

package test

//go:generate wire
import (
	"context"

	"github.com/Sainarasimhan/sample/pkg/endpoints"
	repo "github.com/Sainarasimhan/sample/pkg/repository"
	"github.com/Sainarasimhan/sample/pkg/sample"
	"github.com/google/wire"
)

var (
	TestSet = wire.NewSet(
		sample.BaseDeps,
		wire.InterfaceValue(new(repo.Repository), new(TestRepo)),
		sample.ProvideService,
		sample.ProvideEndpoint,
	)
)

func GetBaseDeps() (deps sample.BaseDependencies, cleanup func(), err error) {
	wire.Build(sample.BaseDeps)
	return deps, cleanup, err
}

func GetTestEndpoint() (te endpoints.Endpoints, cleanup func(), err error) {
	wire.Build(TestSet)
	return te, cleanup, err
}

// test implementation of repo layer
type TestRepo []repo.Details

func (t *TestRepo) Insert(_ context.Context, req repo.Request) (repo.Response, error) {
	*t = append(*t, repo.Details{
		ID:     req.ID,
		Param1: req.Param1,
		Param2: req.Param2,
		Param3: req.Param3,
	})
	return repo.Response{}, nil

}
func (t *TestRepo) Update(_ context.Context, req repo.Request) error {
	for i, d := range *t {
		if d.ID == req.ID {
			newDtl := repo.Details{
				ID:     req.ID,
				Param1: req.Param1,
				Param2: req.Param2,
				Param3: req.Param3,
			}
			(*t)[i] = newDtl
			break
		}
	}
	return nil
}
func (t *TestRepo) List(_ context.Context, req repo.Request) ([]repo.Details, error) {
	// Return details as present in testcase's expected body
	resp := []repo.Details{}
	for _, d := range *t {
		if d.ID == req.ID {
			resp = append(resp, d)
		}
	}
	return resp, nil
}
func (t *TestRepo) Delete(_ context.Context, req repo.Request) error {
	for i, d := range *t {
		if d.ID == req.ID {
			*t = append((*t)[:i], (*t)[(i+1):]...)
		}
	}
	return nil
}

func (t *TestRepo) Close() {
	// Nothing
}
