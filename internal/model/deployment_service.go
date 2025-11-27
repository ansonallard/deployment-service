package model

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"github.com/ansonallard/deployment-service/internal/api"
	"github.com/ansonallard/deployment-service/internal/request"
	"github.com/ansonallard/deployment-service/internal/utils"
)

type Service struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	GitSSHUrl       string `json:"git_ssh_url"`
	GitBranchName   string `json:"branch_name"`
	GitRepoFilePath string
	Configuration   ServiceConfiguration `json:"configuration"`
}

type ServiceConfiguration struct {
	Npm     *NpmConfiguration
	OpenAPI *OpenAPIConfiguration
}

type OpenAPIConfiguration struct {
	OpenAPI *OpenAPIServiceConfiguration
}

type OpenAPIServiceConfiguration struct {
	YamlFile string
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

func (s *Service) FromCreateRequest(req request.Request) error {
	var err error
	var servceInputDto api.CreateServiceRequest
	servceInputDto, err = parseRequestBody[api.CreateServiceRequest](req.GetRequestBody())
	if err != nil {
		return err
	}
	s.Name = servceInputDto.Service.Name

	var gitConfigurationOptions api.GitConfigurationOptions
	if gitConfigurationOptions, err = servceInputDto.Service.Git.AsGitConfigurationOptions(); err != nil {
		return err
	}

	s.GitSSHUrl = gitConfigurationOptions.SshUrl
	s.GitBranchName = gitConfigurationOptions.BranchName

	s.ID = utils.GenerateUlidString()

	serviceConfiguration, err := s.generateServiceConfiguration(servceInputDto.Service.Configuration)
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
	var openapiConfiguration api.OpenAPIConfiguration

	if openapiConfiguration, err = serviceConfig.AsOpenAPIConfiguration(); err != nil {
		if _, ok := err.(*json.SyntaxError); ok {
			return nil, &unionMemberNotPresent{}
		}
		return nil, err
	}

	return &ServiceConfiguration{
		OpenAPI: &OpenAPIConfiguration{
			OpenAPI: &OpenAPIServiceConfiguration{
				YamlFile: openapiConfiguration.Openapi.YamlFile,
			},
		},
	}, nil
}

func (s *Service) FromGetRequest(req request.Request) error {
	pathParams := req.GetPathParams()
	name, ok := pathParams["name"]
	if !ok {
		return fmt.Errorf("name not present in path")
	}
	s.Name = name
	return nil
}

func (s *Service) ToExternal(serviceDto *api.Service) error {
	serviceDto.Id = s.ID
	serviceDto.Name = s.Name
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
	serviceDto.Configuration = api.ServiceConfiguration{}
	serviceDto.Configuration.FromOpenAPIConfiguration(api.OpenAPIConfiguration{
		Openapi: api.OpenAPIConfigurationChoices{
			YamlFile: s.Configuration.OpenAPI.OpenAPI.YamlFile,
		},
	})
}

func parseRequestBody[T any](requestBody io.ReadCloser) (T, error) {
	defer requestBody.Close()

	var apiModel T
	rawInput, err := io.ReadAll(requestBody)
	if err != nil {
		return apiModel, err
	}

	if err := json.Unmarshal(rawInput, &apiModel); err != nil {
		return apiModel, err
	}
	return apiModel, nil
}

func FromListRequest(req request.Request) (maxResults int, nextToken string, err error) {
	queryParams := req.GetQueryParams()
	maxResultsList := queryParams["max_results"]
	nextTokenList := queryParams["next_token"]
	if len(nextTokenList) == 1 {
		nextToken = nextTokenList[0]
	}
	maxResult, err := strconv.ParseInt(maxResultsList[0], 10, 32)
	if err != nil {
		return 0, "", err
	}
	return int(maxResult), nextToken, nil

}
