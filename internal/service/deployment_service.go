package service

import (
	"context"
	"fmt"

	"github.com/ansonallard/deployment-service/internal/ierr"
	"github.com/ansonallard/deployment-service/internal/model"
	"github.com/ansonallard/deployment-service/internal/repo"
)

type DeploymentService interface {
	Create(ctx context.Context, service *model.Service) error
	Get(ctx context.Context, serviceName string) (*model.Service, error)
	List(ctx context.Context) error
}

type DeploymentServiceConfig struct {
	Repo repo.DeploymentService
}

type deploymentService struct {
	repo repo.DeploymentService
}

func NewDeploymentService(config DeploymentServiceConfig) (DeploymentService, error) {
	if config.Repo == nil {
		return nil, fmt.Errorf("repo not set")
	}
	return &deploymentService{repo: config.Repo}, nil
}

func (ds *deploymentService) Create(ctx context.Context, service *model.Service) error {
	service, err := ds.Get(ctx, service.Name)
	if err != nil {
		return err
	}
	if service != nil {
		return &ierr.ConflictError{}
	}
	return ds.repo.Create(ctx, service)
}

func (ds *deploymentService) Get(ctx context.Context, serviceName string) (*model.Service, error) {
	return ds.repo.Get(ctx, serviceName)
}

func (ds *deploymentService) List(ctx context.Context) error {
	return nil
}
