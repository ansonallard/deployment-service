package repo

import (
	"context"

	git "github.com/go-git/go-git/v5"
)

type GitClient interface {
	Clone(ctx context.Context, path string, opts *git.CloneOptions) (*git.Repository, error)
}

func NewGitClient() GitClient {
	return &gitClient{}
}

type gitClient struct{}

func (g *gitClient) Clone(ctx context.Context, path string, opts *git.CloneOptions) (*git.Repository, error) {
	return git.PlainCloneContext(ctx, path, false, opts)
}
