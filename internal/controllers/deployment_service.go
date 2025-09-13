package controllers

import (
	"context"

	"github.com/ansonallard/deployment-service/internal/api"
	"github.com/ansonallard/deployment-service/internal/model"
	"github.com/ansonallard/deployment-service/internal/request"
	"github.com/ansonallard/deployment-service/internal/service"
)

type DeploymentServiceController interface {
	CreateService(ctx context.Context, request request.Request) (*api.CreateServiceResponse, error)
	GetService(ctx context.Context, request request.Request) error
	ListServices(ctx context.Context, request request.Request) error
}

type DeploymentServiceControllerConfig struct {
	Service service.DeploymentService
}

type deploymentServiceController struct {
	service service.DeploymentService
}

func NewDeploymentServiceController(config DeploymentServiceControllerConfig) DeploymentServiceController {
	if config.Service == nil {
		panic("service not set")
	}
	return &deploymentServiceController{
		service: config.Service,
	}
}

func (ds *deploymentServiceController) CreateService(ctx context.Context, request request.Request) (*api.CreateServiceResponse, error) {
	service := new(model.Service)
	if err := service.FromCreateRequest(request); err != nil {
		return nil, err
	}

	// TODO: Business logic

	responseDto := new(api.CreateServiceResponse)
	if err := service.ToExternal(responseDto); err != nil {
		return nil, err
	}
	return responseDto, nil
}
func (ds *deploymentServiceController) GetService(ctx context.Context, request request.Request) error {
	return nil
}

func (ds *deploymentServiceController) ListServices(ctx context.Context, request request.Request) error {
	return nil
}
