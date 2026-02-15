package model

import (
	"encoding/json"
	"fmt"

	"github.com/ansonallard/deployment-service/internal/api"
	"github.com/ansonallard/deployment-service/internal/utils"
)

type Service struct {
	Name
	ID              string `json:"id"`
	GitSSHUrl       string `json:"git_ssh_url"`
	GitBranchName   string `json:"branch_name"`
	GitRepoFilePath string
	Configuration   ServiceConfiguration `json:"configuration"`
}

type Name struct {
	Name string `json:"name"`
}

type ServiceConfiguration struct {
	Npm     *NpmConfiguration
	OpenAPI *OpenAPIConfiguration
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

type NpmServiceConfiguration struct {
	ServieConfiguration
}

type ServieConfiguration struct {
	EnvPath           string
	DockerfilePath    string
	DockerComposePath string
	EnvVars           map[string]any
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
