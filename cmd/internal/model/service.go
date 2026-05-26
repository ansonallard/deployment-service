package model

import (
	"encoding/json"
	"fmt"

	"github.com/ansonallard/deployment-service/cmd/internal/api"
	"github.com/ansonallard/deployment-service/cmd/internal/utils"
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

type ServieConfiguration struct {
	EnvPath           string
	DockerfilePath    string
	DockerComposePath string
	EnvVars           EnvVars
}

type DockerComposeConfiguration struct {
	EnvFiles map[string]EnvVars
}

type EnvVars map[string]any

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

func (s *Service) generateServiceConfiguration(serviceConfig api.ServiceConfiguration) (*ServiceConfiguration, error) {
	var err error

	serviceConfigModel, err := s.handleNpmConfiguration(serviceConfig)
	if err != nil {
		if _, ok := err.(*unionMemberNotPresent); !ok {
			return nil, err
		}
		// If the error type was unionMemberNotPresent, then we continue to the next option
	}
	if serviceConfigModel != nil {
		return serviceConfigModel, nil
	}

	serviceConfigModel, err = s.handleOpenApiconfiugration(serviceConfig)
	if err != nil {
		if _, ok := err.(*unionMemberNotPresent); !ok {
			return nil, err
		}
		// If the error type was unionMemberNotPresent, then we continue to the next option
	}
	if serviceConfigModel != nil {
		return serviceConfigModel, nil
	}

	serviceConfigModel, err = s.handleGoConfiguration(serviceConfig)
	if err != nil {
		if _, ok := err.(*unionMemberNotPresent); !ok {
			return nil, err
		}
		// If the error type was unionMemberNotPresent, then we continue to the next option
	}
	if serviceConfigModel != nil {
		return serviceConfigModel, nil
	}

	serviceConfigModel, err = s.handleDockerComposeConfiguration(serviceConfig)
	if err != nil {
		if _, ok := err.(*unionMemberNotPresent); !ok {
			return nil, err
		}
	}
	if serviceConfigModel != nil {
		return serviceConfigModel, nil
	}

	return nil, fmt.Errorf("invalid config provided")
}

func (s *Service) handleNpmConfiguration(serviceConfig api.ServiceConfiguration) (*ServiceConfiguration, error) {
	var err error
	var npmConfiguration api.NPMConfiguration

	if npmConfiguration, err = serviceConfig.AsNPMConfiguration(); err != nil {
		return nil, err
	}

	var npmServiceConfiguration api.NPMService
	if npmServiceConfiguration, err = npmConfiguration.Npm.AsNPMService(); err != nil {
		if _, ok := err.(*json.SyntaxError); ok {
			return nil, &unionMemberNotPresent{}
		}
		return nil, err
	}

	return &ServiceConfiguration{
		Npm: &NpmConfiguration{
			Service: &NpmServiceConfiguration{
				ServieConfiguration{
					EnvPath:           npmServiceConfiguration.Service.EnvPath,
					DockerfilePath:    npmServiceConfiguration.Service.DockerfilePath,
					DockerComposePath: npmServiceConfiguration.Service.DockerComposePath,
					EnvVars:           npmServiceConfiguration.Service.EnvVars,
				},
			},
		},
	}, nil
}

func (s *Service) handleOpenApiconfiugration(serviceConfig api.ServiceConfiguration) (*ServiceConfiguration, error) {
	var err error
	var openapiConfigurationDto api.OpenAPIConfiguration

	if openapiConfigurationDto, err = serviceConfig.AsOpenAPIConfiguration(); err != nil {
		if _, ok := err.(*json.SyntaxError); ok {
			return nil, &unionMemberNotPresent{}
		}
		return nil, err
	}

	// FIXME: yamlfilepath is a required property, and if it's not set, then we didn't have this type
	if len(openapiConfigurationDto.Openapi.YamlFile) == 0 {
		return nil, &unionMemberNotPresent{}
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
			Name: Name{
				Name: openapiConfigurationDto.Openapi.TypescriptClient.Name,
			},
		}
	}

	if openapiConfigurationDto.Openapi.GoClient != nil {
		internalServiceConfig.OpenAPI.OpenAPI.GoClient = &GoClient{
			Name: Name{
				Name: openapiConfigurationDto.Openapi.GoClient.Name,
			},
		}
	}
	return &internalServiceConfig, nil
}

func (s *Service) handleGoConfiguration(serviceConfig api.ServiceConfiguration) (*ServiceConfiguration, error) {
	var err error
	var goConfigurationDto api.GoConfiguration
	var goService api.GoService

	if goConfigurationDto, err = serviceConfig.AsGoConfiguration(); err != nil {
		if _, ok := err.(*json.SyntaxError); ok {
			return nil, &unionMemberNotPresent{}
		}
		return nil, err
	}

	if goService, err = goConfigurationDto.Go.AsGoService(); err != nil {
		if _, ok := err.(*json.SyntaxError); ok {
			return nil, &unionMemberNotPresent{}
		}
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
	default:
		return fmt.Errorf("invalid service configuration")
	}

	return nil
}

func (s *Service) toNpmExternal(serviceDto *api.Service) {
	npmConfiguration := api.NPMConfigurationChoices{}
	npmConfiguration.FromNPMService(api.NPMService{
		Service: api.NPMServiceConfiguration{
			DockerComposePath: s.Configuration.Npm.Service.DockerComposePath,
			DockerfilePath:    s.Configuration.Npm.Service.DockerfilePath,
			EnvPath:           s.Configuration.Npm.Service.EnvPath,
			EnvVars:           s.Configuration.Npm.Service.EnvVars,
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

func (s *Service) handleDockerComposeConfiguration(serviceConfig api.ServiceConfiguration) (*ServiceConfiguration, error) {
	dockerComposeConfig, err := serviceConfig.AsDockerComposeConfiguration()
	if err != nil {
		if _, ok := err.(*json.SyntaxError); ok {
			return nil, &unionMemberNotPresent{}
		}
		return nil, err
	}

	internalConfig := DockerComposeConfiguration{}

	if dockerComposeConfig.DockerCompose.EnvFiles != nil {
		internalConfig.EnvFiles = make(map[string]EnvVars)
		for k, v := range *dockerComposeConfig.DockerCompose.EnvFiles {
			internalConfig.EnvFiles[k] = EnvVars(v)
		}
	}

	return &ServiceConfiguration{
		DockerCompose: &internalConfig,
	}, nil
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
