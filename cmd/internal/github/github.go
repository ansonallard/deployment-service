package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/go-github/v66/github"
	"golang.org/x/oauth2"
)

type GitHubClient interface {
	EnsureGithubRepoExists(ctx context.Context, repoName string) error
	GetOwner() string
	GetToken() string
}

type githubClient struct {
	client *github.Client
	owner  string
	token  string
}

func NewGithubClient(ctx context.Context, token, owner string) GitHubClient {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	return &githubClient{
		client: github.NewClient(oauth2.NewClient(ctx, ts)),
		owner:  owner,
		token:  token,
	}
}

func (gc *githubClient) GetOwner() string {
	return gc.owner
}

func (gc *githubClient) GetToken() string {
	return gc.token
}

// ensureGithubRepoExists checks for repoName under the configured owner and
// creates it (private) if missing. Repo creation only sets up the shell —
// the actual Docker build step pushes the first commit + tag into it.
func (gc *githubClient) EnsureGithubRepoExists(ctx context.Context, repoName string) error {
	_, resp, err := gc.client.Repositories.Get(ctx, gc.owner, repoName)
	if err == nil {
		return nil // already exists
	}
	if resp == nil || resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("failed to check for github repo %q: %w", repoName, err)
	}

	// org param must be "" if Owner is a user account rather than an org.
	newRepo := &github.Repository{
		Name:    github.String(repoName),
		Private: github.Bool(true),
	}

	// Only supports users, not orgs.
	if _, _, err := gc.client.Repositories.Create(ctx, "", newRepo); err != nil {
		var ghErr *github.ErrorResponse
		if errors.As(err, &ghErr) && ghErr.Response.StatusCode == http.StatusUnprocessableEntity {
			return nil // race with a concurrent build creating the same repo
		}
		return fmt.Errorf("failed to create github repo %q: %w", repoName, err)
	}
	return nil
}
