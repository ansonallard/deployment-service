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
	GitRepoOrigin  string
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
		filePath:                 config.ServiceFilPath,
		serviceConfigurationFile: "service_definition.json",
		gitRepoPath:              "repo",
		sshKeyPath:               config.SSHKeyPath,
		gitClient:                config.GitClient,
		gitRepoOrigin:            config.GitRepoOrigin,
	}, nil
}

type deploymentService struct {
	filePath                 string
	serviceConfigurationFile string
	gitRepoPath              string
	sshKeyPath               string
	gitClient                GitClient
	gitRepoOrigin            string
}

func (ds *deploymentService) Create(ctx context.Context, service *model.Service) error {
	servicePath := ds.getServiceFilePath(service.Name)

	if err := os.MkdirAll(servicePath, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	fileBytes, err := json.MarshalIndent(service, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal service: %w", err)
	}

	if err := os.WriteFile(ds.getServiceConfigurationFilePath(service.Name), fileBytes, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	gitRepoPath := ds.getGitRepoFilePath(service.Name)
	if err := os.MkdirAll(gitRepoPath, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	service.GitRepoFilePath = gitRepoPath

	sshAuth, err := ssh.NewPublicKeysFromFile("git", ds.sshKeyPath, "")
	if err != nil {
		return fmt.Errorf("failed to load ssh key: %w", err)
	}

	_, err = ds.gitClient.Clone(ctx, gitRepoPath, &git.CloneOptions{
		URL:           service.GitSSHUrl,
		ReferenceName: plumbing.ReferenceName(service.GitBranchName),
		SingleBranch:  true,
		Auth:          sshAuth,
		RemoteName:    ds.gitRepoOrigin,
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
	servicePath := ds.getServiceFilePath(serviceName)
	file, err := os.Stat(servicePath)
	if err != nil {
		return nil, &ierr.NotFoundError{}
	}
	if !file.IsDir() {
		return nil, &ierr.NotFoundError{}
	}
	fileBytes, err := os.ReadFile(ds.getServiceConfigurationFilePath(serviceName))
	if err != nil {
		return nil, err
	}
	service := new(model.Service)
	if err := json.Unmarshal(fileBytes, service); err != nil {
		return nil, err
	}
	service.GitRepoFilePath = ds.getGitRepoFilePath(serviceName)
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

func (ds *deploymentService) getServiceFilePath(serviceName string) string {
	return path.Join(ds.filePath, serviceName)
}

func (ds *deploymentService) getGitRepoFilePath(serviceName string) string {
	return path.Join(ds.getServiceFilePath(serviceName), ds.gitRepoPath)
}

func (ds *deploymentService) getServiceConfigurationFilePath(serviceName string) string {
	return path.Join(ds.getServiceFilePath(serviceName), ds.serviceConfigurationFile)
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
