package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"

	"github.com/ansonallard/deployment-service/cmd/internal/model"
	"github.com/ansonallard/deployment-service/cmd/internal/utils"
	"github.com/ansonallard/go_utils/openapi/ierr"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("deployment-service.repo")

type DeploymentService interface {
	Create(ctx context.Context, service *model.Service) error
	Get(ctx context.Context, serviceName string) (*model.Service, error)
	List(ctx context.Context, maxResults int, nextToken string) ([]*model.Service, error)
	Update(ctx context.Context, name string, ifMatch string, partial *model.Service) (*model.Service, error)
	Delete(ctx context.Context, serviceName string) error
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
	ctx, span := tracer.Start(ctx, "repo.create",
		trace.WithAttributes(attribute.String("service.name", service.Name.Name)),
	)
	defer span.End()

	servicePath := ds.getServiceFilePath(service.Name.Name)

	if err := os.MkdirAll(servicePath, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	fileBytes, err := json.MarshalIndent(service, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal service: %w", err)
	}

	if err := os.WriteFile(ds.getServiceConfigurationFilePath(service.Name.Name), fileBytes, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	gitRepoPath := ds.getGitRepoFilePath(service.Name.Name)
	if err := os.MkdirAll(gitRepoPath, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	service.GitRepoFilePath = gitRepoPath

	sshAuth, err := ssh.NewPublicKeysFromFile("git", ds.sshKeyPath, "")
	if err != nil {
		return fmt.Errorf("failed to load ssh key: %w", err)
	}

	cloneCtx, cloneSpan := tracer.Start(ctx, "repo.git.clone",
		trace.WithAttributes(
			attribute.String("service.name", service.Name.Name),
			attribute.String("git.url", service.GitSSHUrl),
			attribute.String("git.branch", service.GitBranchName),
		),
	)
	_, err = ds.gitClient.Clone(cloneCtx, gitRepoPath, &git.CloneOptions{
		URL:           service.GitSSHUrl,
		ReferenceName: plumbing.ReferenceName(service.GitBranchName),
		SingleBranch:  true,
		Auth:          sshAuth,
		RemoteName:    ds.gitRepoOrigin,
	})
	if err != nil {
		cloneSpan.RecordError(err)
		cloneSpan.SetStatus(codes.Error, err.Error())
	}
	cloneSpan.End()
	if err != nil {
		if err := os.RemoveAll(servicePath); err != nil {
			return fmt.Errorf("failed to clean up service file")
		}
		return fmt.Errorf("failed to clone repo: %w", err)
	}

	return nil
}

func (ds *deploymentService) Get(ctx context.Context, serviceName string) (*model.Service, error) {
	ctx, span := tracer.Start(ctx, "repo.get",
		trace.WithAttributes(attribute.String("service.name", serviceName)),
	)
	defer span.End()

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
	ctx, span := tracer.Start(ctx, "repo.list",
		trace.WithAttributes(attribute.Int("max_results", maxResults)),
	)
	defer span.End()

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

func (ds *deploymentService) Update(ctx context.Context, name string, ifMatch string, partial *model.Service) (*model.Service, error) {
	ctx, span := tracer.Start(ctx, "repo.update",
		trace.WithAttributes(
			attribute.String("service.name", name),
		),
	)
	defer span.End()

	current, err := ds.Get(ctx, name)
	if err != nil {
		return nil, err
	}

	if current.Version != ifMatch {
		return nil, &model.PreConditionFailedError{}
	}

	current.Configuration = partial.Configuration
	current.Version = utils.GenerateUlidString()

	fileBytes, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal service: %w", err)
	}

	if err := os.WriteFile(ds.getServiceConfigurationFilePath(name), fileBytes, 0644); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	current.GitRepoFilePath = ds.getGitRepoFilePath(name)
	return current, nil
}

func (ds *deploymentService) Delete(ctx context.Context, serviceName string) error {
	ctx, span := tracer.Start(ctx, "repo.delete",
		trace.WithAttributes(attribute.String("service.name", serviceName)),
	)
	defer span.End()

	servicePath := ds.getServiceFilePath(serviceName)
	if _, err := os.Stat(servicePath); err != nil {
		return &ierr.NotFoundError{}
	}
	if err := os.RemoveAll(servicePath); err != nil {
		return fmt.Errorf("failed to delete service: %w", err)
	}
	return nil
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
