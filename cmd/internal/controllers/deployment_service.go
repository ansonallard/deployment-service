package controllers

import (
	"context"
	"fmt"

	"github.com/ansonallard/deployment-service/cmd/internal/model"
	"github.com/ansonallard/deployment-service/cmd/internal/service"
	"github.com/ansonallard/deployment_service_go_client/lib/deployment_service_go_client"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("deployment-service.controllers")

type DeploymentServiceController interface {
	// (GET /services)
	ListServices(ctx context.Context, request deployment_service_go_client.ListServicesRequestObject) (deployment_service_go_client.ListServicesResponseObject, error)

	// (POST /services)
	CreateService(ctx context.Context, request deployment_service_go_client.CreateServiceRequestObject) (deployment_service_go_client.CreateServiceResponseObject, error)

	// (GET /services/{name})
	GetService(ctx context.Context, request deployment_service_go_client.GetServiceRequestObject) (deployment_service_go_client.GetServiceResponseObject, error)

	// (PUT /services/{name})
	UpdateService(ctx context.Context, request deployment_service_go_client.UpdateServiceRequestObject) (deployment_service_go_client.UpdateServiceResponseObject, error)

	// (DELETE /services/{name})
	DeleteService(ctx context.Context, request deployment_service_go_client.DeleteServiceRequestObject) (deployment_service_go_client.DeleteServiceResponseObject, error)
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

func (ds *deploymentServiceController) CreateService(ctx context.Context, request deployment_service_go_client.CreateServiceRequestObject) (deployment_service_go_client.CreateServiceResponseObject, error) {
	ctx, span := tracer.Start(ctx, "controllers.create",
		trace.WithAttributes(attribute.String("service.name", request.Body.Service.Name)),
	)
	defer span.End()

	service := new(model.Service)
	if err := service.FromCreateRequest(request.Body); err != nil {
		return nil, err
	}
	if err := ds.service.Create(ctx, service); err != nil {
		return nil, err
	}
	serviceDto := new(deployment_service_go_client.Service)
	if err := service.ToExternal(serviceDto); err != nil {
		return nil, err
	}
	version := deployment_service_go_client.Version(service.Version)
	return deployment_service_go_client.CreateService200JSONResponse{
		Body: deployment_service_go_client.CreateServiceResponse{
			Service: *serviceDto,
		},
		Headers: deployment_service_go_client.CreateService200ResponseHeaders{
			ETag: &version,
		},
	}, nil
}

func (ds *deploymentServiceController) GetService(ctx context.Context, request deployment_service_go_client.GetServiceRequestObject) (deployment_service_go_client.GetServiceResponseObject, error) {
	ctx, span := tracer.Start(ctx, "controllers.get",
		trace.WithAttributes(attribute.String("service.name", string(request.Name))),
	)
	defer span.End()

	service, err := ds.service.Get(ctx, request.Name)
	if err != nil {
		return nil, err
	}

	serviceDto := new(deployment_service_go_client.Service)
	if err := service.ToExternal(serviceDto); err != nil {
		return nil, err
	}
	version := deployment_service_go_client.Version(service.Version)
	return deployment_service_go_client.GetService200JSONResponse{
		Body: deployment_service_go_client.GetServiceResponse{
			Service: *serviceDto,
		},
		Headers: deployment_service_go_client.GetService200ResponseHeaders{
			ETag: &version,
		},
	}, nil
}

func (ds *deploymentServiceController) ListServices(ctx context.Context, request deployment_service_go_client.ListServicesRequestObject) (deployment_service_go_client.ListServicesResponseObject, error) {
	ctx, span := tracer.Start(ctx, "controllers.list")
	defer span.End()

	maxResults, nextToken := model.FromListRequest(request.Params)

	services, err := ds.service.List(ctx, maxResults, nextToken)
	if err != nil {
		return nil, err
	}

	servicesDto := make(deployment_service_go_client.Services, 0)

	for _, service := range services {
		serviceDto := new(deployment_service_go_client.Service)
		if err := service.ToExternal(serviceDto); err != nil {
			return nil, err
		}
		servicesDto = append(servicesDto, *serviceDto)
	}

	return deployment_service_go_client.ListServices200JSONResponse{
		Services:  servicesDto,
		NextToken: nil,
	}, nil
}

func (ds *deploymentServiceController) UpdateService(ctx context.Context, request deployment_service_go_client.UpdateServiceRequestObject) (deployment_service_go_client.UpdateServiceResponseObject, error) {
	ctx, span := tracer.Start(ctx, "controllers.update",
		trace.WithAttributes(attribute.String("service.name", string(request.Name))),
	)
	defer span.End()

	partial := new(model.Service)
	if err := partial.FromUpdateRequest(request.Body); err != nil {
		return nil, err
	}

	updated, err := ds.service.Update(ctx, request.Name, string(request.Params.IfMatch), partial)
	if err != nil {
		return nil, err
	}

	serviceDto := new(deployment_service_go_client.Service)
	if err := updated.ToExternal(serviceDto); err != nil {
		return nil, err
	}

	version := deployment_service_go_client.Version(updated.Version)
	return deployment_service_go_client.UpdateService200JSONResponse{
		Body: deployment_service_go_client.UpdateServiceResponse{
			Service: *serviceDto,
		},
		Headers: deployment_service_go_client.UpdateService200ResponseHeaders{
			ETag: &version,
		},
	}, nil
}

func (ds *deploymentServiceController) DeleteService(ctx context.Context, request deployment_service_go_client.DeleteServiceRequestObject) (deployment_service_go_client.DeleteServiceResponseObject, error) {
	ctx, span := tracer.Start(ctx, "controllers.delete",
		trace.WithAttributes(attribute.String("service.name", string(request.Name))),
	)
	defer span.End()

	if err := ds.service.Delete(ctx, request.Name); err != nil {
		return nil, err
	}
	return deployment_service_go_client.DeleteService204Response{}, nil
}
