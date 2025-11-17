package model

import (
	"encoding/json"
	"errors"
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
	FilePath        string                 `json:"file_path"`
	ClientLibraries []OpenAPIClientLibrary `json:"client_libraries"`
}

type OpenAPIClientLibrary struct {
	NpmClientLibrary *NpmClientLibrary `json:"npm_client_lib"`
	GoClientLibrary  *GoClientLibrary  `json:"go_client_lib"`
}

type NpmClientLibrary struct {
	ClientLibrary
}
type GoClientLibrary struct {
	ClientLibrary
}

type ClientLibrary struct {
	Name string `json:"name"`
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
	var serviceInputDto api.CreateServiceRequest
	serviceInputDto, err = parseRequestBody[api.CreateServiceRequest](req.GetRequestBody())
	if err != nil {
		return err
	}
	s.Name = serviceInputDto.Service.Name

	var gitConfigurationOptions api.GitConfigurationOptions
	if gitConfigurationOptions, err = serviceInputDto.Service.Git.AsGitConfigurationOptions(); err != nil {
		return err
	}

	s.GitSSHUrl = gitConfigurationOptions.SshUrl
	s.GitBranchName = gitConfigurationOptions.BranchName

	s.ID = utils.GenerateUlidString()

	configuration := serviceInputDto.Service.Configuration
	if npmConfigDto, err := configuration.AsNPMConfiguration(); err != nil {
		npmConfig, err := s.fromNPMConfiguration(npmConfigDto)
		if err != nil {
			return err
		}
		s.Configuration = ServiceConfiguration{
			Npm: npmConfig,
		}
	} else if openAPIConfig, err := configuration.AsOpenAPIConfiguration(); err != nil {
		openAPIConfig, err := s.fromOpenAPIConfiguration(openAPIConfig)
		if err != nil {
			return err
		}
		s.Configuration = ServiceConfiguration{
			OpenAPI: openAPIConfig,
		}

	} else {
		return fmt.Errorf("invalid service configuration provided")
	}

	return nil
}

func (s *Service) fromOpenAPIConfiguration(openAPIConfiguration api.OpenAPIConfiguration) (*OpenAPIConfiguration, error) {
	var openAPIServiceConfiguration api.OpenAPIConfigurationChoices
	clientLibraries := make([]OpenAPIClientLibrary, len(openAPIConfiguration.Openapi.ClientLibraries))

	var errs []error
	for i, clientLibDto := range openAPIConfiguration.Openapi.ClientLibraries {

		if npmLibraryConfig, err := clientLibDto.AsNpmClientLibraryConfiguration(); err != nil {
			clientLibraries[i] = OpenAPIClientLibrary{
				NpmClientLibrary: &NpmClientLibrary{ClientLibrary: ClientLibrary{Name: *npmLibraryConfig.Name}},
			}
		} else if goLibraryConfig, err := clientLibDto.AsGoClientLibraryConfiguration(); err != nil {
			clientLibraries[i] = OpenAPIClientLibrary{
				GoClientLibrary: &GoClientLibrary{ClientLibrary: ClientLibrary{Name: *goLibraryConfig.Name}},
			}
		} else {
			errs = append(errs, err)
			continue
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return &OpenAPIConfiguration{
		FilePath:        openAPIServiceConfiguration.OpenAPIDefinitionPath,
		ClientLibraries: clientLibraries,
	}, nil
}

func (s *Service) fromNPMConfiguration(npmConfiguration api.NPMConfiguration) (*NpmConfiguration, error) {
	var err error
	var npmServiceConfiguration api.NPMService
	if npmServiceConfiguration, err = npmConfiguration.Npm.AsNPMService(); err != nil {
		return nil, err
	}
	return &NpmConfiguration{
		Service: &NpmServiceConfiguration{
			ServieConfiguration{
				EnvPath:           npmServiceConfiguration.Service.EnvPath,
				DockerfilePath:    npmServiceConfiguration.Service.DockerfilePath,
				DockerComposePath: npmServiceConfiguration.Service.DockerComposePath,
				EnvVars:           npmServiceConfiguration.Service.EnvVars,
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

func (s *Service) ToExternal(res *api.Service) error {
	res.Id = s.ID
	res.Name = s.Name
	res.Git = api.GitConfiguration{}
	if err := res.Git.FromGitConfigurationOptions(api.GitConfigurationOptions{
		SshUrl:     s.GitSSHUrl,
		BranchName: s.GitBranchName,
	}); err != nil {
		return err
	}

	switch {
	case s.Configuration.Npm != nil:
		s.toExternalNpmConfiguration(res)
	case s.Configuration.OpenAPI != nil:
		s.toExternalOpenAPIConfiguration(res)
	}

	return nil
}

func (s *Service) toExternalNpmConfiguration(res *api.Service) {
	npmConfiguration := api.NPMConfigurationChoices{}
	npmConfiguration.FromNPMService(api.NPMService{
		Service: api.NPMServiceConfiguration{
			DockerComposePath: s.Configuration.Npm.Service.DockerComposePath,
			DockerfilePath:    s.Configuration.Npm.Service.DockerfilePath,
			EnvPath:           s.Configuration.Npm.Service.EnvPath,
			EnvVars:           s.Configuration.Npm.Service.EnvVars,
		},
	})
	res.Configuration = api.ServiceConfiguration{}
	res.Configuration.FromNPMConfiguration(api.NPMConfiguration{
		Npm: npmConfiguration,
	})
}

func (s *Service) toExternalOpenAPIConfiguration(res *api.Service) {
	openAPIConfiguration := api.OpenAPIConfigurationChoices{}
	openAPIConfiguration.OpenAPIDefinitionPath = s.Configuration.OpenAPI.FilePath

	for i, clientLib := range s.Configuration.OpenAPI.ClientLibraries {
		config := api.OpenAPIClientLibraryConfiguration{}
		switch {
		case clientLib.GoClientLibrary != nil:
			config.FromGoClientLibraryConfiguration(api.GoClientLibraryConfiguration{
				Name: &clientLib.GoClientLibrary.Name,
			})
		case clientLib.NpmClientLibrary != nil:
			config.FromNpmClientLibraryConfiguration(api.NpmClientLibraryConfiguration{
				Name: &clientLib.GoClientLibrary.Name,
			})
		}
		openAPIConfiguration.ClientLibraries[i] = config
	}

	res.Configuration = api.ServiceConfiguration{}
	res.Configuration.FromOpenAPIConfiguration(api.OpenAPIConfiguration{
		Openapi: openAPIConfiguration,
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
