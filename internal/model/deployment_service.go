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
	ID            string               `json:"id"`
	Name          string               `json:"name"`
	GitSSHUrl     string               `json:"git_ssh_url"`
	GitBranchName string               `json:"branch_name"`
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

	var gitConfigurationOptions api.GitConfigurationOptions
	if gitConfigurationOptions, err = servceInputDto.Service.Git.AsGitConfigurationOptions(); err != nil {
		return err
	}

	s.GitSSHUrl = gitConfigurationOptions.SshUrl
	s.GitBranchName = gitConfigurationOptions.BranchName

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
	if err := res.Git.FromGitConfigurationOptions(api.GitConfigurationOptions{
		SshUrl:     s.GitSSHUrl,
		BranchName: s.GitBranchName,
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
