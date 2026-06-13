package service

import (
	"context"
	"fmt"

	"github.com/ansonallard/deployment-service/cmd/internal/model"
	"github.com/ansonallard/deployment-service/cmd/internal/repo"
	"github.com/ansonallard/go_utils/openapi/ierr"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("deployment-service.service")

type DeploymentService interface {
	Create(ctx context.Context, service *model.Service) error
	Get(ctx context.Context, serviceName string) (*model.Service, error)
	List(ctx context.Context, maxResults int, nextToken string) ([]*model.Service, error)
	Update(ctx context.Context, name string, ifMatch string, partial *model.Service) (*model.Service, error)
	Delete(ctx context.Context, serviceName string) error
	CollectExistingServicesForBackgroundProcessing(ctx context.Context) error
}

type ServiceChannel = chan string

type DeploymentServiceConfig struct {
	Repo                 repo.DeploymentService
	BackgroundJobChannel ServiceChannel
}

type deploymentService struct {
	repo                 repo.DeploymentService
	backgroundJobChannel ServiceChannel
}

func NewDeploymentService(config DeploymentServiceConfig) (DeploymentService, error) {
	if config.Repo == nil {
		return nil, fmt.Errorf("repo not set")
	}
	if config.BackgroundJobChannel == nil {
		return nil, fmt.Errorf("background channel not set")
	}
	return &deploymentService{repo: config.Repo, backgroundJobChannel: config.BackgroundJobChannel}, nil
}

func (ds *deploymentService) Create(ctx context.Context, service *model.Service) error {
	ctx, span := tracer.Start(ctx, "service.create",
		trace.WithAttributes(attribute.String("service.name", service.Name.Name)),
	)
	defer span.End()

	existingService, err := ds.Get(ctx, service.Name.Name)
	if err != nil {
		switch err.(type) {
		case *ierr.NotFoundError:
			// Continue on
		default:
			return err
		}
	}
	if existingService != nil {
		return &ierr.ConflictError{}
	}
	err = ds.repo.Create(ctx, service)
	if err != nil {
		return err
	}
	// Kick off background processing
	ds.backgroundJobChannel <- service.Name.Name
	return nil
}

func (ds *deploymentService) Get(ctx context.Context, serviceName string) (*model.Service, error) {
	ctx, span := tracer.Start(ctx, "service.get",
		trace.WithAttributes(attribute.String("service.name", serviceName)),
	)
	defer span.End()

	return ds.repo.Get(ctx, serviceName)
}

func (ds *deploymentService) List(ctx context.Context, maxResults int, nextToken string) ([]*model.Service, error) {
	ctx, span := tracer.Start(ctx, "service.list",
		trace.WithAttributes(attribute.Int("max_results", maxResults)),
	)
	defer span.End()

	return ds.repo.List(ctx, maxResults, nextToken)
}

func (ds *deploymentService) Update(ctx context.Context, name string, ifMatch string, partial *model.Service) (*model.Service, error) {
	ctx, span := tracer.Start(ctx, "service.update",
		trace.WithAttributes(attribute.String("service.name", name)),
	)
	defer span.End()

	return ds.repo.Update(ctx, name, ifMatch, partial)
}

func (ds *deploymentService) Delete(ctx context.Context, serviceName string) error {
	ctx, span := tracer.Start(ctx, "service.delete",
		trace.WithAttributes(attribute.String("service.name", serviceName)),
	)
	defer span.End()

	return ds.repo.Delete(ctx, serviceName)
}

func (ds *deploymentService) CollectExistingServicesForBackgroundProcessing(ctx context.Context) error {
	ctx, span := tracer.Start(ctx, "service.collect_existing")
	defer span.End()

	services, err := ds.List(ctx, 100, "")
	if err != nil {
		return err
	}
	log := zerolog.Ctx(ctx)
	log.Info().Interface("services", services).Int("numberOfServices", len(services)).
		Msgf("Collected %d pre-existing services. Sending notifications for processing", len(services))
	for _, service := range services {
		ds.backgroundJobChannel <- service.Name.Name
		log.Info().Interface("service", service).Msg("Notified for processing")
	}
	return nil
}
