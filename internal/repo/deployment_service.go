package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"

	"github.com/ansonallard/deployment-service/internal/ierr"
	"github.com/ansonallard/deployment-service/internal/model"
)

type DeploymentService interface {
	Create(ctx context.Context, service *model.Service) error
	Get(ctx context.Context, serviceName string) (*model.Service, error)
}

func NewDeploymentService(serviceFilePath string) (DeploymentService, error) {
	if serviceFilePath == "" {
		return nil, fmt.Errorf("serviceFilePath not set")
	}
	if err := dirExists(serviceFilePath); err != nil {
		return nil, err
	}
	return &deploymentService{
		filePath:              serviceFilePath,
		serviceDefinitionFile: "service_definition.json",
	}, nil
}

func dirExists(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("path isn't a directory")
	}
	return nil
}

type deploymentService struct {
	filePath              string
	serviceDefinitionFile string
}

func (ds *deploymentService) Create(ctx context.Context, service *model.Service) error {
	servicePath := path.Join(ds.filePath, service.Name)

	if err := os.MkdirAll(servicePath, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	fileBytes, err := json.MarshalIndent(service, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal service: %w", err)
	}

	// Define file path inside the directory
	filePath := path.Join(servicePath, ds.serviceDefinitionFile)

	// Write file
	if err := os.WriteFile(filePath, fileBytes, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

func (ds *deploymentService) Get(ctx context.Context, serviceName string) (*model.Service, error) {
	servicePath := path.Join(ds.filePath, serviceName)
	file, err := os.Stat(servicePath)
	if err != nil {
		return nil, &ierr.NotFoundError{}
	}
	if !file.IsDir() {
		return nil, &ierr.NotFoundError{}
	}
	fileBytes, err := os.ReadFile(path.Join(servicePath, ds.serviceDefinitionFile))
	if err != nil {
		return nil, err
	}
	service := new(model.Service)
	if err := json.Unmarshal(fileBytes, service); err != nil {
		return nil, err
	}
	return service, nil
}
