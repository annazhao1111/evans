package usecase

import (
	"context"
	"io"

	"github.com/ktr0731/evans/entity"
	"github.com/ktr0731/evans/entity/env"
	"github.com/ktr0731/evans/usecase/pbusecase"
	"github.com/ktr0731/evans/usecase/port"
)

// TODO: remove dependency related to pbusecase
// usecase package should not be used from another packages.
// instead of it, we should use pbusecase.

type Interactor struct {
	env env.Environment

	outputPort     port.OutputPort
	inputterPort   port.Inputter
	grpcPort       entity.GRPCClient
	dynamicBuilder port.DynamicBuilder
}

type InteractorParams struct {
	Env env.Environment

	OutputPort     port.OutputPort
	InputterPort   port.Inputter
	DynamicBuilder port.DynamicBuilder
	GRPCClient     entity.GRPCClient
}

func (p *InteractorParams) Cleanup(ctx context.Context) error {
	if p.GRPCClient != nil {
		return p.GRPCClient.Close(ctx)
	}
	return nil
}

func NewInteractor(params *InteractorParams) *Interactor {
	return &Interactor{
		env:            params.Env,
		outputPort:     params.OutputPort,
		inputterPort:   params.InputterPort,
		grpcPort:       params.GRPCClient,
		dynamicBuilder: params.DynamicBuilder,
	}
}

func (i *Interactor) Package(params *port.PackageParams) (io.Reader, error) {
	return pbusecase.Package(params, i.outputPort, i.env)
}

func (i *Interactor) Service(params *port.ServiceParams) (io.Reader, error) {
	return Service(params, i.outputPort, i.env)
}

func (i *Interactor) Describe(params *port.DescribeParams) (io.Reader, error) {
	return pbusecase.Describe(params, i.outputPort, i.env)
}

func (i *Interactor) Show(params *port.ShowParams) (io.Reader, error) {
	return pbusecase.Show(params, i.outputPort, i.env)
}

func (i *Interactor) Header(params *port.HeaderParams) (io.Reader, error) {
	return Header(params, i.outputPort, i.env)
}

func (i *Interactor) Call(params *port.CallParams) (io.Reader, error) {
	return Call(params, i.outputPort, i.inputterPort, i.grpcPort, i.dynamicBuilder, i.env)
}
