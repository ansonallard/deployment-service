package model

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/ansonallard/deployment-service/internal/api"
	"github.com/ansonallard/deployment-service/internal/request"
	"github.com/ansonallard/deployment-service/internal/utils"
)

type Service struct {
	ID            string               `json:"id"`
	Name          string               `json:"name"`
	GitSSHUrl     string               `json:"git_ssh_url"`
	Configuration ServiceConfiguration `json:"configuration"`
}

type ServiceConfiguration struct {
	Npm *NpmConfiguration
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
}

func (s *Service) FromCreateRequest(req request.Request) error {
	var err error
	var servceInputDto api.CreateServiceRequest
	servceInputDto, err = parseRequestBody[api.CreateServiceRequest](req.GetRequestBody())
	if err != nil {
		return err
	}
	s.Name = servceInputDto.Service.Name

	var gitSshUrl api.GitSSHURL
	if gitSshUrl, err = servceInputDto.Service.Git.AsGitSSHURL(); err != nil {
		return err
	}

	s.GitSSHUrl = gitSshUrl.SshUrl

	s.ID = utils.GenerateUlidString()

	var npmConfiguration api.NPMConfiguration
	if npmConfiguration, err = servceInputDto.Service.Configuration.AsNPMConfiguration(); err != nil {
		return err
	}

	var npmServiceConfiguration api.NPMService
	if npmServiceConfiguration, err = npmConfiguration.Npm.AsNPMService(); err != nil {
		return err
	}

	s.Configuration = ServiceConfiguration{
		Npm: &NpmConfiguration{
			Service: &NpmServiceConfiguration{
				ServieConfiguration{
					EnvPath:           npmServiceConfiguration.Service.EnvPath,
					DockerfilePath:    npmServiceConfiguration.Service.DockerfilePath,
					DockerComposePath: npmServiceConfiguration.Service.DockerComposePath,
				},
			},
		},
	}
	return nil
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
	if err := res.Git.FromGitSSHURL(api.GitSSHURL{
		SshUrl: s.GitSSHUrl,
	}); err != nil {
		return err
	}

	npmConfiguration := api.NPMConfigurationChoices{}
	npmConfiguration.FromNPMService(api.NPMService{
		Service: api.NPMServiceConfiguration{
			DockerComposePath: s.Configuration.Npm.Service.DockerComposePath,
			DockerfilePath:    s.Configuration.Npm.Service.DockerfilePath,
			EnvPath:           s.Configuration.Npm.Service.EnvPath,
		},
	})
	res.Configuration = api.ServiceConfiguration{}
	res.Configuration.FromNPMConfiguration(api.NPMConfiguration{
		Npm: npmConfiguration,
	})
	return nil
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
