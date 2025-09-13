package controllers

import (
	"context"

	"github.com/ansonallard/deployment-service/internal/api"
	"github.com/ansonallard/deployment-service/internal/request"
)

type DeploymentControllers interface {
	CreateService(ctx context.Context, request request.Request) (api.CreateServiceResponse, error)
	GetService(ctx context.Context, request request.Request) error
	ListServices(ctx context.Context, request request.Request) error
}

type deploymentController struct{}

func NewDeploymentControllers() DeploymentControllers {
	return &deploymentController{}
}

func (ds *deploymentController) CreateService(ctx context.Context, request request.Request) (api.CreateServiceResponse, error) {
	return api.CreateServiceResponse{
		Service: api.Service{
			Id: "01234567890123456789012345",
		},
	}, nil
}
func (ds *deploymentController) GetService(ctx context.Context, request request.Request) error {
	return nil
}

func (ds *deploymentController) ListServices(ctx context.Context, request request.Request) error {
	return nil
}
