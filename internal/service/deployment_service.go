package service

import "context"

type DeploymentService interface {
	Create(ctx context.Context) error
	Get(ctx context.Context) error
	List(ctx context.Context) error
}

type deploymentService struct{}

func NewDeploymentService() DeploymentService {
	return &deploymentService{}
}

func (ds *deploymentService) Create(ctx context.Context) error {
	return nil
}
func (ds *deploymentService) Get(ctx context.Context) error {
	return nil
}
func (ds *deploymentService) List(ctx context.Context) error {
	return nil
}
