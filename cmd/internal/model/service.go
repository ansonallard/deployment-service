package model

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ansonallard/deployment-service/cmd/internal/api"
	"github.com/ansonallard/deployment-service/cmd/internal/utils"
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

type GoClient struct {
	Name
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
	EnvFiles map[string]EnvVars
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

func (s *Service) FromCreateRequest(dto *api.CreateServiceRequest) error {
	var err error
	s.Name.Name = dto.Service.Name

	var gitConfigurationOptions api.GitConfigurationOptions
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

func (s *Service) FromUpdateRequest(dto *api.UpdateServiceRequest) error {
	serviceConfiguration, err := s.generateServiceConfiguration(dto.Service.Configuration)
	if err != nil {
		return err
	}
	s.Configuration = *serviceConfiguration
	return nil
}

func (s *Service) ToExternal(serviceDto *api.Service) error {
	serviceDto.Id = s.ID
	serviceDto.Name = s.Name.Name
	serviceDto.Git = api.GitConfiguration{}
	if err := serviceDto.Git.FromGitConfigurationOptions(api.GitConfigurationOptions{
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

func (s *Service) toNpmExternal(serviceDto *api.Service) {
	npmConfiguration := api.NPMConfigurationChoices{}
	var serviceType api.NPMServiceType
	switch s.Configuration.Npm.Service.ServiceType {
	case NpmServiceTypeBackend:
		serviceType = api.Backend
	case NpmServiceTypeFrontend:
		serviceType = api.Frontend
	default:
		serviceType = api.Backend
	}

	npmConfiguration.FromNPMService(api.NPMService{
		Service: api.NPMServiceConfiguration{
			Type: serviceType,
		},
	})
	serviceDto.Configuration = api.ServiceConfiguration{}
	serviceDto.Configuration.FromNPMConfiguration(api.NPMConfiguration{
		Npm: npmConfiguration,
	})
}

func (s *Service) toOpenApiExternal(serviceDto *api.Service) {
	openapiConfig := api.OpenAPIConfigurationChoices{
		YamlFile: s.Configuration.OpenAPI.OpenAPI.YamlFile,
	}

	if s.Configuration.OpenAPI.OpenAPI.TypescriptClient != nil {
		openapiConfig.TypescriptClient = &api.OpenAPITypescriptClientConfig{
			Name: s.Configuration.OpenAPI.OpenAPI.TypescriptClient.Name.Name,
		}
	}

	if s.Configuration.OpenAPI.OpenAPI.GoClient != nil {
		openapiConfig.GoClient = &api.OpenAPIGoClientConfig{
			Name: s.Configuration.OpenAPI.OpenAPI.GoClient.Name.Name,
		}
	}

	serviceDto.Configuration = api.ServiceConfiguration{}
	serviceDto.Configuration.FromOpenAPIConfiguration(api.OpenAPIConfiguration{
		Openapi: openapiConfig,
	})
}

func (s *Service) toGoExternal(serviceDto *api.Service) {
	serviceConfig := api.GoServiceConfiguration{}
	if s.Configuration.Go.Service.BinaryDirectory != "" {
		serviceConfig.BinaryDirectory = &s.Configuration.Go.Service.BinaryDirectory
	}

	goConfiguration := api.GoConfigurationChoices{}
	goConfiguration.FromGoService(api.GoService{
		Service: serviceConfig,
	})
	serviceDto.Configuration = api.ServiceConfiguration{}
	serviceDto.Configuration.FromGoConfiguration(api.GoConfiguration{
		Go: goConfiguration,
	})
}

func FromListRequest(params api.ListServicesParams) (maxResults int, nextToken string) {
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

func (s *Service) toDockerBuildExternal(serviceDto *api.Service) {
	serviceDto.Configuration = api.ServiceConfiguration{}
	serviceDto.Configuration.FromDockerBuildConfiguration(api.DockerBuildConfiguration{
		DockerBuild: api.DockerBuildConfigurationOptions{
			DockerfilePath: &s.Configuration.DockerBuild.DockerfilePath,
		},
	})
}

func (s *Service) toDockerComposeExternal(serviceDto *api.Service) {
	envFiles := make(api.EnvFiles)
	for k, v := range s.Configuration.DockerCompose.EnvFiles {
		envFiles[k] = api.EnvVars(v)
	}

	serviceDto.Configuration = api.ServiceConfiguration{}
	serviceDto.Configuration.FromDockerComposeConfiguration(api.DockerComposeConfiguration{
		DockerCompose: api.DockerComposeConfigurationOptions{
			EnvFiles: &envFiles,
		},
	})
}

func (s *Service) generateServiceConfiguration(serviceConfig api.ServiceConfiguration) (*ServiceConfiguration, error) {
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

func (s *Service) handleNpmConfiguration(serviceConfig api.ServiceConfiguration) (*ServiceConfiguration, error) {
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
		case api.Backend:
			serviceType = NpmServiceTypeBackend
		case api.Frontend:
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

func (s *Service) handleOpenApiconfiugration(serviceConfig api.ServiceConfiguration) (*ServiceConfiguration, error) {
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
			Name: Name{Name: openapiConfigurationDto.Openapi.GoClient.Name},
		}
	}

	return &internalServiceConfig, nil
}

func (s *Service) handleGoConfiguration(serviceConfig api.ServiceConfiguration) (*ServiceConfiguration, error) {
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

func (s *Service) handleDockerComposeConfiguration(serviceConfig api.ServiceConfiguration) (*ServiceConfiguration, error) {
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

	return &ServiceConfiguration{DockerCompose: &internalConfig}, nil
}

func (s *Service) handleDockerBuildConfiguration(serviceConfig api.ServiceConfiguration) (*ServiceConfiguration, error) {
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
