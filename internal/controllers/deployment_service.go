package controllers

import (
	"context"
	"fmt"

	"github.com/ansonallard/deployment-service/internal/api"
	"github.com/ansonallard/deployment-service/internal/model"
	"github.com/ansonallard/deployment-service/internal/service"
)

type DeploymentServiceController interface {
	// (GET /services)
	ListServices(ctx context.Context, request api.ListServicesRequestObject) (api.ListServicesResponseObject, error)

	// (POST /services)
	CreateService(ctx context.Context, request api.CreateServiceRequestObject) (api.CreateServiceResponseObject, error)

	// (GET /services/{name})
	GetService(ctx context.Context, request api.GetServiceRequestObject) (api.GetServiceResponseObject, error)
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

func (ds *deploymentServiceController) CreateService(ctx context.Context, request api.CreateServiceRequestObject) (api.CreateServiceResponseObject, error) {
	service := new(model.Service)
	if err := service.FromCreateRequest(request.Body); err != nil {
		return nil, err
	}
	if err := ds.service.Create(ctx, service); err != nil {
		return nil, err
	}
	serviceDto := new(api.Service)
	if err := service.ToExternal(serviceDto); err != nil {
		return nil, err
	}
	return api.CreateService200JSONResponse{
		Service: *serviceDto,
	}, nil
}

func (ds *deploymentServiceController) GetService(ctx context.Context, request api.GetServiceRequestObject) (api.GetServiceResponseObject, error) {
	service, err := ds.service.Get(ctx, request.Name)
	if err != nil {
		return nil, err
	}

	serviceDto := new(api.Service)
	if err := service.ToExternal(serviceDto); err != nil {
		return nil, err
	}
	return api.GetService200JSONResponse{
		Service: *serviceDto,
	}, nil
}

func (ds *deploymentServiceController) ListServices(ctx context.Context, request api.ListServicesRequestObject) (api.ListServicesResponseObject, error) {
	maxResults, nextToken := model.FromListRequest(request.Params)

	services, err := ds.service.List(ctx, maxResults, nextToken)
	if err != nil {
		return nil, err
	}

	servicesDto := make(api.Services, 0)

	for _, service := range services {
		serviceDto := new(api.Service)
		if err := service.ToExternal(serviceDto); err != nil {
			return nil, err
		}
		servicesDto = append(servicesDto, *serviceDto)
	}

	return api.ListServices200JSONResponse{
		Services:  servicesDto,
		NextToken: nil,
	}, nil
}
