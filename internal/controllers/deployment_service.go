package controllers

import (
	"context"
	"fmt"

	"github.com/ansonallard/deployment-service/internal/api"
	"github.com/ansonallard/deployment-service/internal/model"
	"github.com/ansonallard/deployment-service/internal/request"
	"github.com/ansonallard/deployment-service/internal/service"
)

type DeploymentServiceController interface {
	CreateService(ctx context.Context, request request.Request) (*api.CreateServiceResponse, error)
	GetService(ctx context.Context, request request.Request) (*api.GetServiceResponse, error)
	ListServices(ctx context.Context, request request.Request) error
}

type DeploymentServiceControllerConfig struct {
	Service service.DeploymentService
}

type deploymentServiceController struct {
	service service.DeploymentService
}

func NewDeploymentServiceController(config DeploymentServiceControllerConfig) (DeploymentServiceController, error) {
	if config.Service == nil {
		return nil, fmt.Errorf("service not set")
	}
	return &deploymentServiceController{
		service: config.Service,
	}, nil
}

func (ds *deploymentServiceController) CreateService(ctx context.Context, request request.Request) (*api.CreateServiceResponse, error) {
	service := new(model.Service)
	if err := service.FromCreateRequest(request); err != nil {
		return nil, err
	}

	if err := ds.service.Create(ctx, service); err != nil {
		return nil, err
	}

	serviceDto := new(api.Service)
	if err := service.ToExternal(serviceDto); err != nil {
		return nil, err
	}
	return &api.CreateServiceResponse{
		Service: *serviceDto,
	}, nil
}

func (ds *deploymentServiceController) GetService(ctx context.Context, request request.Request) (*api.GetServiceResponse, error) {
	service := new(model.Service)
	if err := service.FromGetRequest(request); err != nil {
		return nil, err
	}

	service, err := ds.service.Get(ctx, service.Name)
	if err != nil {
		return nil, err
	}

	serviceDto := new(api.Service)
	if err := service.ToExternal(serviceDto); err != nil {
		return nil, err
	}
	return &api.GetServiceResponse{
		Service: *serviceDto,
	}, nil
}

func (ds *deploymentServiceController) ListServices(ctx context.Context, request request.Request) error {
	return nil
}
