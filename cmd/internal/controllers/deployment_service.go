package controllers

import (
	"context"
	"fmt"

	"github.com/ansonallard/deployment-service/cmd/internal/api"
	"github.com/ansonallard/deployment-service/cmd/internal/model"
	"github.com/ansonallard/deployment-service/cmd/internal/service"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("deployment-service.controllers")

type DeploymentServiceController interface {
	// (GET /services)
	ListServices(ctx context.Context, request api.ListServicesRequestObject) (api.ListServicesResponseObject, error)

	// (POST /services)
	CreateService(ctx context.Context, request api.CreateServiceRequestObject) (api.CreateServiceResponseObject, error)

	// (GET /services/{name})
	GetService(ctx context.Context, request api.GetServiceRequestObject) (api.GetServiceResponseObject, error)

	// (PUT /services/{name})
	UpdateService(ctx context.Context, request api.UpdateServiceRequestObject) (api.UpdateServiceResponseObject, error)

	// (DELETE /services/{name})
	DeleteService(ctx context.Context, request api.DeleteServiceRequestObject) (api.DeleteServiceResponseObject, error)
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
	serviceDto := new(api.Service)
	if err := service.ToExternal(serviceDto); err != nil {
		return nil, err
	}
	return api.CreateService200JSONResponse{
		Body: api.CreateServiceResponse{
			Service: *serviceDto,
		},
		Headers: api.CreateService200ResponseHeaders{
			ETag: api.Version(service.Version),
		},
	}, nil
}

func (ds *deploymentServiceController) GetService(ctx context.Context, request api.GetServiceRequestObject) (api.GetServiceResponseObject, error) {
	ctx, span := tracer.Start(ctx, "controllers.get",
		trace.WithAttributes(attribute.String("service.name", string(request.Name))),
	)
	defer span.End()

	service, err := ds.service.Get(ctx, request.Name)
	if err != nil {
		return nil, err
	}

	serviceDto := new(api.Service)
	if err := service.ToExternal(serviceDto); err != nil {
		return nil, err
	}
	return api.GetService200JSONResponse{
		Body: api.GetServiceResponse{
			Service: *serviceDto,
		},
		Headers: api.GetService200ResponseHeaders{
			ETag: api.Version(service.Version),
		},
	}, nil
}

func (ds *deploymentServiceController) ListServices(ctx context.Context, request api.ListServicesRequestObject) (api.ListServicesResponseObject, error) {
	ctx, span := tracer.Start(ctx, "controllers.list")
	defer span.End()

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

func (ds *deploymentServiceController) UpdateService(ctx context.Context, request api.UpdateServiceRequestObject) (api.UpdateServiceResponseObject, error) {
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

	serviceDto := new(api.Service)
	if err := updated.ToExternal(serviceDto); err != nil {
		return nil, err
	}

	return api.UpdateService200JSONResponse{
		Body: api.UpdateServiceResponse{
			Service: *serviceDto,
		},
		Headers: api.UpdateService200ResponseHeaders{
			ETag: api.Version(updated.Version),
		},
	}, nil
}

func (ds *deploymentServiceController) DeleteService(ctx context.Context, request api.DeleteServiceRequestObject) (api.DeleteServiceResponseObject, error) {
	ctx, span := tracer.Start(ctx, "controllers.delete",
		trace.WithAttributes(attribute.String("service.name", string(request.Name))),
	)
	defer span.End()

	if err := ds.service.Delete(ctx, request.Name); err != nil {
		return nil, err
	}
	return api.DeleteService204Response{}, nil
}
