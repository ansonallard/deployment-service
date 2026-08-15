package model

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ansonallard/deployment-service/cmd/internal/utils"
	"github.com/ansonallard/deployment_service_go_client/lib/deployment_service_go_client"
	"github.com/ansonallard/go_utils/openapi/ierr"
)

type Service struct {
	Name
	ID              string `json:"id"`
	Version         string `json:"version"`
	GitSSHUrl       string `json:"git_ssh_url"`
	GitBranchName   string `json:"branch_name"`
	GitRepoFilePath string
	Configuration   ServiceConfiguration `json:"configuration"`
}

type Name struct {
	Name string `json:"name"`
}

type ServiceConfiguration struct {
	Npm           *NpmConfiguration
	OpenAPI       *OpenAPIConfiguration
	Go            *GoConfiguration
	DockerCompose *DockerComposeConfiguration
	DockerBuild   *DockerBuildConfiguration
}

type OpenAPIConfiguration struct {
	OpenAPI *OpenAPIServiceConfiguration
}

type OpenAPIServiceConfiguration struct {
	YamlFile         string
	TypescriptClient *TypescriptClient
	GoClient         *GoClient
}

type TypescriptClient struct {
	Name
}

type RemotePackageRegistry int

const (
	PrivateRegistry RemotePackageRegistry = iota
	Github
)

type GoClient struct {
	Name
	Registry RemotePackageRegistry
}

type NpmConfiguration struct {
	Service *NpmServiceConfiguration
}

type GoConfiguration struct {
	Service *GoServiceConfiguration
}

type GoServiceConfiguration struct {
	BinaryDirectory string
}

type NpmServiceConfiguration struct {
	ServieConfiguration
}

type NpmServiceType string

const (
	NpmServiceTypeBackend  = "backend"
	NpmServiceTypeFrontend = "frontend"
)

type ServieConfiguration struct {
	ServiceType NpmServiceType
}

type DockerComposeConfiguration struct {
	EnvFiles      map[string]EnvVars
	RefreshImages bool
}

type DockerBuildConfiguration struct {
	DockerfilePath string
}

type EnvVars map[string]any

type serviceConfigMember string
type npmConfigMember string
type goConfigMember string

const (
	serviceConfigNpm           serviceConfigMember = "npm"
	serviceConfigOpenAPI       serviceConfigMember = "openapi"
	serviceConfigGo            serviceConfigMember = "go"
	serviceConfigDockerCompose serviceConfigMember = "dockerCompose"
	serviceConfigDockerBuild   serviceConfigMember = "dockerBuild"
)

const (
	npmConfigService npmConfigMember = "service"
	npmConfigLibrary npmConfigMember = "library"
)

const (
	goConfigService goConfigMember = "service"
)

var serviceConfigurationMembers = []serviceConfigMember{
	serviceConfigNpm,
	serviceConfigOpenAPI,
	serviceConfigGo,
	serviceConfigDockerCompose,
	serviceConfigDockerBuild,
}

var npmConfigurationMembers = []npmConfigMember{
	npmConfigService,
	npmConfigLibrary,
}

var goConfigurationMembers = []goConfigMember{
	goConfigService,
}

type unionMember interface {
	~string
}

func detectUnionMember[T unionMember](raw []byte, members []T) (T, error) {
	var probe map[string]any
	if err := json.Unmarshal(raw, &probe); err != nil {
		var zero T
		return zero, err
	}

	present := []T{}
	for _, member := range members {
		if _, ok := probe[string(member)]; ok {
			present = append(present, member)
		}
	}

	if len(present) == 0 {
		strs := make([]string, len(members))
		for i, m := range members {
			strs[i] = string(m)
		}
		var zero T
		return zero, ierr.NewBadRequestError(fmt.Sprintf("invalid configuration: none of the expected fields present, expected one of: %s", strings.Join(strs, ", ")))
	}
	if len(present) > 1 {
		strs := make([]string, len(present))
		for i, m := range present {
			strs[i] = string(m)
		}
		var zero T
		return zero, ierr.NewBadRequestError(fmt.Sprintf("invalid configuration: multiple configuration types provided: %s", strings.Join(strs, ", ")))
	}
	return present[0], nil
}

func (s *Service) FromCreateRequest(dto *deployment_service_go_client.CreateServiceRequest) error {
	var err error
	s.Name.Name = dto.Service.Name

	var gitConfigurationOptions deployment_service_go_client.GitConfigurationOptions
	if gitConfigurationOptions, err = dto.Service.Git.AsGitConfigurationOptions(); err != nil {
		return err
	}

	s.GitSSHUrl = gitConfigurationOptions.SshUrl
	s.GitBranchName = gitConfigurationOptions.BranchName

	s.ID = utils.GenerateUlidString()
	s.Version = utils.GenerateUlidString()

	serviceConfiguration, err := s.generateServiceConfiguration(dto.Service.Configuration)
	if err != nil {
		return err
	}
	s.Configuration = *serviceConfiguration
	return nil
}

func (s *Service) FromUpdateRequest(dto *deployment_service_go_client.UpdateServiceRequest) error {
	serviceConfiguration, err := s.generateServiceConfiguration(dto.Service.Configuration)
	if err != nil {
		return err
	}
	s.Configuration = *serviceConfiguration
	return nil
}

func (s *Service) ToExternal(serviceDto *deployment_service_go_client.Service) error {
	serviceDto.Id = s.ID
	serviceDto.Name = s.Name.Name
	serviceDto.Git = deployment_service_go_client.GitConfiguration{}
	if err := serviceDto.Git.FromGitConfigurationOptions(deployment_service_go_client.GitConfigurationOptions{
		SshUrl:     s.GitSSHUrl,
		BranchName: s.GitBranchName,
	}); err != nil {
		return err
	}

	switch {
	case s.Configuration.Npm != nil:
		s.toNpmExternal(serviceDto)
	case s.Configuration.OpenAPI != nil:
		s.toOpenApiExternal(serviceDto)
	case s.Configuration.Go != nil:
		s.toGoExternal(serviceDto)
	case s.Configuration.DockerCompose != nil:
		s.toDockerComposeExternal(serviceDto)
	case s.Configuration.DockerBuild != nil:
		s.toDockerBuildExternal(serviceDto)
	default:
		return ierr.NewBadRequestError("invalid service configuration")
	}

	return nil
}

func (s *Service) toNpmExternal(serviceDto *deployment_service_go_client.Service) {
	npmConfiguration := deployment_service_go_client.NPMConfigurationChoices{}
	var serviceType deployment_service_go_client.NPMServiceType
	switch s.Configuration.Npm.Service.ServiceType {
	case NpmServiceTypeBackend:
		serviceType = deployment_service_go_client.Backend
	case NpmServiceTypeFrontend:
		serviceType = deployment_service_go_client.Frontend
	default:
		serviceType = deployment_service_go_client.Backend
	}

	npmConfiguration.FromNPMService(deployment_service_go_client.NPMService{
		Service: deployment_service_go_client.NPMServiceConfiguration{
			Type: serviceType,
		},
	})
	serviceDto.Configuration = deployment_service_go_client.ServiceConfiguration{}
	serviceDto.Configuration.FromNPMConfiguration(deployment_service_go_client.NPMConfiguration{
		Npm: npmConfiguration,
	})
}

func (s *Service) toOpenApiExternal(serviceDto *deployment_service_go_client.Service) {
	openapiConfig := deployment_service_go_client.OpenAPIConfigurationChoices{
		YamlFile: s.Configuration.OpenAPI.OpenAPI.YamlFile,
	}

	if s.Configuration.OpenAPI.OpenAPI.TypescriptClient != nil {
		openapiConfig.TypescriptClient = &deployment_service_go_client.OpenAPITypescriptClientConfig{
			Name: s.Configuration.OpenAPI.OpenAPI.TypescriptClient.Name.Name,
		}
	}

	if s.Configuration.OpenAPI.OpenAPI.GoClient != nil {
		openapiConfig.GoClient = &deployment_service_go_client.OpenAPIGoClientConfig{
			Name:     s.Configuration.OpenAPI.OpenAPI.GoClient.Name.Name,
			Registry: s.Configuration.OpenAPI.OpenAPI.GoClient.Registry.toExternal(),
		}
	}

	serviceDto.Configuration = deployment_service_go_client.ServiceConfiguration{}
	serviceDto.Configuration.FromOpenAPIConfiguration(deployment_service_go_client.OpenAPIConfiguration{
		Openapi: openapiConfig,
	})
}

func (r RemotePackageRegistry) toExternal() *deployment_service_go_client.OpenAPIGoClientConfigRegistry {
	switch r {
	case Github:
		r := deployment_service_go_client.Github
		return &r
	case PrivateRegistry:
		r := deployment_service_go_client.PrivateRegistry
		return &r
	default:
		r := deployment_service_go_client.PrivateRegistry
		return &r
	}
}

func fromOpenAPIGoClientConfigRegistryExternal(r *deployment_service_go_client.OpenAPIGoClientConfigRegistry) RemotePackageRegistry {
	if r == nil {
		return PrivateRegistry
	}
	switch *r {
	case deployment_service_go_client.PrivateRegistry:
		return PrivateRegistry
	case deployment_service_go_client.Github:
		return Github
	default:
		return PrivateRegistry
	}
}

func (s *Service) toGoExternal(serviceDto *deployment_service_go_client.Service) {
	serviceConfig := deployment_service_go_client.GoServiceConfiguration{}
	if s.Configuration.Go.Service.BinaryDirectory != "" {
		serviceConfig.BinaryDirectory = &s.Configuration.Go.Service.BinaryDirectory
	}

	goConfiguration := deployment_service_go_client.GoConfigurationChoices{}
	goConfiguration.FromGoService(deployment_service_go_client.GoService{
		Service: serviceConfig,
	})
	serviceDto.Configuration = deployment_service_go_client.ServiceConfiguration{}
	serviceDto.Configuration.FromGoConfiguration(deployment_service_go_client.GoConfiguration{
		Go: goConfiguration,
	})
}

func FromListRequest(params deployment_service_go_client.ListServicesParams) (maxResults int, nextToken string) {
	maxResults = 100
	nextToken = ""
	if params.MaxResults != nil {
		maxResults = *params.MaxResults
	}
	if params.NextToken != nil {
		nextToken = *params.NextToken
	}
	return maxResults, nextToken
}

func (s *Service) toDockerBuildExternal(serviceDto *deployment_service_go_client.Service) {
	serviceDto.Configuration = deployment_service_go_client.ServiceConfiguration{}
	serviceDto.Configuration.FromDockerBuildConfiguration(deployment_service_go_client.DockerBuildConfiguration{
		DockerBuild: deployment_service_go_client.DockerBuildConfigurationOptions{
			DockerfilePath: &s.Configuration.DockerBuild.DockerfilePath,
		},
	})
}

func (s *Service) toDockerComposeExternal(serviceDto *deployment_service_go_client.Service) {
	var envFiles deployment_service_go_client.EnvFiles
	if s.Configuration.DockerCompose.EnvFiles != nil {
		envFiles = make(deployment_service_go_client.EnvFiles)
		for k, v := range s.Configuration.DockerCompose.EnvFiles {
			envFiles[k] = deployment_service_go_client.EnvVars(v)
		}
	}

	serviceDto.Configuration = deployment_service_go_client.ServiceConfiguration{}
	serviceDto.Configuration.FromDockerComposeConfiguration(deployment_service_go_client.DockerComposeConfiguration{
		DockerCompose: deployment_service_go_client.DockerComposeConfigurationOptions{
			EnvFiles:      &envFiles,
			RefreshImages: &s.Configuration.DockerCompose.RefreshImages,
		},
	})
}

func (s *Service) generateServiceConfiguration(serviceConfig deployment_service_go_client.ServiceConfiguration) (*ServiceConfiguration, error) {
	raw, err := serviceConfig.MarshalJSON()
	if err != nil {
		return nil, err
	}

	member, err := detectUnionMember(raw, serviceConfigurationMembers)
	if err != nil {
		return nil, err
	}

	switch member {
	case serviceConfigNpm:
		return s.handleNpmConfiguration(serviceConfig)
	case serviceConfigOpenAPI:
		return s.handleOpenApiconfiugration(serviceConfig)
	case serviceConfigGo:
		return s.handleGoConfiguration(serviceConfig)
	case serviceConfigDockerCompose:
		return s.handleDockerComposeConfiguration(serviceConfig)
	case serviceConfigDockerBuild:
		return s.handleDockerBuildConfiguration(serviceConfig)
	default:
		return nil, ierr.NewBadRequestError(fmt.Sprintf("unhandled configuration type: %s", member))
	}
}

func (s *Service) handleNpmConfiguration(serviceConfig deployment_service_go_client.ServiceConfiguration) (*ServiceConfiguration, error) {
	npmConfiguration, err := serviceConfig.AsNPMConfiguration()
	if err != nil {
		return nil, err
	}

	raw, err := npmConfiguration.Npm.MarshalJSON()
	if err != nil {
		return nil, err
	}

	member, err := detectUnionMember(raw, npmConfigurationMembers)
	if err != nil {
		return nil, err
	}

	switch member {
	case npmConfigService:
		service, err := npmConfiguration.Npm.AsNPMService()
		if err != nil {
			return nil, err
		}

		var serviceType NpmServiceType
		switch service.Service.Type {
		case deployment_service_go_client.Backend:
			serviceType = NpmServiceTypeBackend
		case deployment_service_go_client.Frontend:
			serviceType = NpmServiceTypeFrontend
		}

		return &ServiceConfiguration{
			Npm: &NpmConfiguration{
				Service: &NpmServiceConfiguration{
					ServieConfiguration{
						ServiceType: serviceType,
					},
				},
			},
		}, nil
	case npmConfigLibrary:
		// TODO: handle library case
		return nil, ierr.NewBadRequestError("npm library configuration not yet implemented")
	default:
		return nil, ierr.NewBadRequestError(fmt.Sprintf("unhandled npm configuration type: %s", member))
	}
}

func (s *Service) handleOpenApiconfiugration(serviceConfig deployment_service_go_client.ServiceConfiguration) (*ServiceConfiguration, error) {
	openapiConfigurationDto, err := serviceConfig.AsOpenAPIConfiguration()
	if err != nil {
		return nil, err
	}

	// yamlFile is a required property that distinguishes this type structurally
	if len(openapiConfigurationDto.Openapi.YamlFile) == 0 {
		return nil, ierr.NewBadRequestError("invalid openapi configuration: yamlFile is required")
	}

	internalServiceConfig := ServiceConfiguration{
		OpenAPI: &OpenAPIConfiguration{
			OpenAPI: &OpenAPIServiceConfiguration{
				YamlFile: openapiConfigurationDto.Openapi.YamlFile,
			},
		},
	}

	if openapiConfigurationDto.Openapi.TypescriptClient != nil {
		internalServiceConfig.OpenAPI.OpenAPI.TypescriptClient = &TypescriptClient{
			Name: Name{Name: openapiConfigurationDto.Openapi.TypescriptClient.Name},
		}
	}

	if openapiConfigurationDto.Openapi.GoClient != nil {
		internalServiceConfig.OpenAPI.OpenAPI.GoClient = &GoClient{
			Name:     Name{Name: openapiConfigurationDto.Openapi.GoClient.Name},
			Registry: fromOpenAPIGoClientConfigRegistryExternal(openapiConfigurationDto.Openapi.GoClient.Registry),
		}
	}

	return &internalServiceConfig, nil
}

func (s *Service) handleGoConfiguration(serviceConfig deployment_service_go_client.ServiceConfiguration) (*ServiceConfiguration, error) {
	goConfigurationDto, err := serviceConfig.AsGoConfiguration()
	if err != nil {
		return nil, err
	}

	raw, err := goConfigurationDto.Go.MarshalJSON()
	if err != nil {
		return nil, err
	}

	member, err := detectUnionMember(raw, goConfigurationMembers)
	if err != nil {
		return nil, err
	}

	switch member {
	case goConfigService:
		goService, err := goConfigurationDto.Go.AsGoService()
		if err != nil {
			return nil, err
		}

		internalGoServiceConfig := GoServiceConfiguration{}
		if goService.Service.BinaryDirectory != nil && *goService.Service.BinaryDirectory != "" {
			internalGoServiceConfig.BinaryDirectory = *goService.Service.BinaryDirectory
		}

		return &ServiceConfiguration{
			Go: &GoConfiguration{
				Service: &internalGoServiceConfig,
			},
		}, nil
	default:
		return nil, ierr.NewBadRequestError(fmt.Sprintf("unhandled go configuration type: %s", member))
	}
}

func (s *Service) handleDockerComposeConfiguration(serviceConfig deployment_service_go_client.ServiceConfiguration) (*ServiceConfiguration, error) {
	dockerComposeConfig, err := serviceConfig.AsDockerComposeConfiguration()
	if err != nil {
		return nil, err
	}

	internalConfig := DockerComposeConfiguration{}
	if dockerComposeConfig.DockerCompose.EnvFiles != nil {
		internalConfig.EnvFiles = make(map[string]EnvVars)
		for k, v := range *dockerComposeConfig.DockerCompose.EnvFiles {
			internalConfig.EnvFiles[k] = EnvVars(v)
		}
	}
	if dockerComposeConfig.DockerCompose.RefreshImages != nil {
		internalConfig.RefreshImages = *dockerComposeConfig.DockerCompose.RefreshImages
	}

	return &ServiceConfiguration{DockerCompose: &internalConfig}, nil
}

func (s *Service) handleDockerBuildConfiguration(serviceConfig deployment_service_go_client.ServiceConfiguration) (*ServiceConfiguration, error) {
	dockerBuildConfig, err := serviceConfig.AsDockerBuildConfiguration()
	if err != nil {
		return nil, err
	}

	internalConfig := DockerBuildConfiguration{}
	if dockerBuildConfig.DockerBuild.DockerfilePath != nil && *dockerBuildConfig.DockerBuild.DockerfilePath != "" {
		internalConfig.DockerfilePath = *dockerBuildConfig.DockerBuild.DockerfilePath
	}

	return &ServiceConfiguration{DockerBuild: &internalConfig}, nil
}
