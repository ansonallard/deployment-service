package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"

	"github.com/ansonallard/deployment-service/internal/ierr"
	"github.com/ansonallard/deployment-service/internal/model"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
)

type DeploymentService interface {
	Create(ctx context.Context, service *model.Service) error
	Get(ctx context.Context, serviceName string) (*model.Service, error)
	List(ctx context.Context, maxResults int, nextToken string) ([]*model.Service, error)
}

type DeploymentServieConfig struct {
	ServiceFilPath string
	SSHKeyPath     string
	GitClient      GitClient
}

func NewDeploymentService(config DeploymentServieConfig) (DeploymentService, error) {
	if config.ServiceFilPath == "" {
		return nil, fmt.Errorf("serviceFilePath not set")
	}
	if config.SSHKeyPath == "" {
		return nil, fmt.Errorf("sshKeyPath not set")
	}
	if config.GitClient == nil {
		return nil, fmt.Errorf("git client not set")
	}
	if err := dirExists(config.ServiceFilPath); err != nil {
		return nil, err
	}
	return &deploymentService{
		filePath:              config.ServiceFilPath,
		serviceDefinitionFile: "service_definition.json",
		gitRepoPath:           "repo",
		sshKeyPath:            config.SSHKeyPath,
		gitClient:             config.GitClient,
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
	gitRepoPath           string
	sshKeyPath            string
	gitClient             GitClient
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

	gitRepoPath := path.Join(servicePath, ds.gitRepoPath)
	if err := os.MkdirAll(gitRepoPath, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Load your private key
	sshAuth, err := ssh.NewPublicKeysFromFile("git", ds.sshKeyPath, "")
	if err != nil {
		return fmt.Errorf("failed to load ssh key: %w", err)
	}

	_, err = ds.gitClient.Clone(ctx, gitRepoPath, &git.CloneOptions{
		URL:           service.GitSSHUrl,
		ReferenceName: plumbing.ReferenceName(service.GitBranchName),
		SingleBranch:  true,
		Auth:          sshAuth,
	})
	if err != nil {
		if err := os.RemoveAll(servicePath); err != nil {
			return fmt.Errorf("failed to clean up service file")
		}
		return fmt.Errorf("failed to clone repo: %w", err)
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

func (ds *deploymentService) List(ctx context.Context, maxResults int, nextToken string) ([]*model.Service, error) {
	services := make([]*model.Service, 0)

	dirEntries, err := os.ReadDir(ds.filePath)
	if err != nil {
		return nil, err
	}
	counter := 0
	for _, dirEntry := range dirEntries {
		if !dirEntry.IsDir() {
			continue
		}
		serviceName := dirEntry.Name()
		service, err := ds.Get(ctx, serviceName)
		if err != nil {
			return nil, err
		}
		counter++
		services = append(services, service)
		if counter >= maxResults {
			break
		}
	}
	return services, nil
}
