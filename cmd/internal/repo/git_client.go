package repo

import (
	"context"

	git "github.com/go-git/go-git/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var gitTracer = otel.Tracer("deployment-service.repo.git")

type GitClient interface {
	Clone(ctx context.Context, path string, opts *git.CloneOptions) (*git.Repository, error)
}

func NewGitClient() GitClient {
	return &gitClient{}
}

type gitClient struct{}

func (g *gitClient) Clone(ctx context.Context, path string, opts *git.CloneOptions) (*git.Repository, error) {
	ctx, span := gitTracer.Start(ctx, "git.clone",
		trace.WithAttributes(
			attribute.String("git.url", opts.URL),
			attribute.String("git.branch", string(opts.ReferenceName)),
		),
	)
	defer span.End()

	repo, err := git.PlainCloneContext(ctx, path, false, opts)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return repo, err
}
