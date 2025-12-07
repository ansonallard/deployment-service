package backgroundprocessor

import (
	"context"
	"fmt"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/ansonallard/deployment-service/internal/background_processor/npm"
	"github.com/ansonallard/deployment-service/internal/background_processor/openapi"
	"github.com/ansonallard/deployment-service/internal/model"
	"github.com/ansonallard/deployment-service/internal/version"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/rs/zerolog"
)

const (
	ciCommitMsgFormat = "ci: Release version %s"
	defaultOrigin     = "origin"
)

type BackgroundProcesseror interface {
	ProcessService(ctx context.Context, service *model.Service) error
}

type BackgroundProcessorConfig struct {
	Versioner           *version.Versioner
	SSHKeyPath          string
	GitRepoOrigin       string
	CiCommitAuthor      *CiCommitAuthor
	NpmServiceProcessor npm.NPMServiceProcessor
	OpenAPIProcessor    openapi.OpenAPIProcessor
}

type CiCommitAuthor struct {
	Name  string
	Email string
}

func NewBackgroundProcessor(config BackgroundProcessorConfig) (BackgroundProcesseror, error) {
	if config.Versioner == nil {
		return nil, fmt.Errorf("versioner not provided")
	}
	sshAuth, err := ssh.NewPublicKeysFromFile("git", config.SSHKeyPath, "")
	if err != nil {
		return nil, fmt.Errorf("failed to load ssh key: %w", err)
	}
	if config.SSHKeyPath == "" {
		return nil, fmt.Errorf("sshKeyPath not provided")
	}
	if config.GitRepoOrigin == "" {
		return nil, fmt.Errorf("gitRepoOrigin not provided")
	}
	if config.CiCommitAuthor == nil {
		return nil, fmt.Errorf("ciCommitAuthor not provided")
	}
	if config.NpmServiceProcessor == nil {
		return nil, fmt.Errorf("npmServiceProcessor not provided")
	}
	if config.OpenAPIProcessor == nil {
		return nil, fmt.Errorf("openAPIProcessor not provided")
	}

	return &backgroundProcessor{
			versioner:           *config.Versioner,
			gitRepoOrigin:       config.GitRepoOrigin,
			sshAuth:             sshAuth,
			ciCommmitAuthor:     config.CiCommitAuthor,
			npmServiceProcessor: config.NpmServiceProcessor,
			openAPIProcessor:    config.OpenAPIProcessor,
		},
		nil
}

type backgroundProcessor struct {
	versioner           version.Versioner
	sshAuth             *ssh.PublicKeys
	gitRepoOrigin       string
	ciCommmitAuthor     *CiCommitAuthor
	npmServiceProcessor npm.NPMServiceProcessor
	openAPIProcessor    openapi.OpenAPIProcessor
}

func (bp *backgroundProcessor) ProcessService(ctx context.Context, service *model.Service) error {
	log := zerolog.Ctx(ctx)

	shouldProcess, err := bp.shouldProcess(ctx, service)
	if err != nil {
		return err
	}
	if !shouldProcess {
		return nil
	}

	nextVersion, err := bp.versioner.CalculateNextVersion(ctx, service.GitRepoFilePath)
	if err != nil {
		return err
	}
	log.Info().Interface("semver", nextVersion).Str("nextVersion", nextVersion.String()).Msg("Next version")

	serviceConfiguration := service.Configuration
	switch {
	case serviceConfiguration.Npm != nil:
		log.Info().Str("service", service.Name.Name).Str("nextVersion", nextVersion.String()).Msg("Npm Service")
		if err := bp.npmServiceProcessor.SetPackageJsonVersion(service, nextVersion); err != nil {
			return err
		}
	case serviceConfiguration.OpenAPI != nil:
		log.Info().Str("service", service.Name.Name).Str("nextVersion", nextVersion.String()).Msg("OpenAPI Service")
		if err := bp.openAPIProcessor.SetOpenApiYamlVersion(service, nextVersion); err != nil {
			return err
		}
	}

	log.Info().Str("service", service.Name.Name).Str("nextVersion", nextVersion.String()).Msg("Commiting changes")
	if err := bp.commitChanges(service.GitRepoFilePath, nextVersion); err != nil {
		return err
	}

	log.Info().Str("service", service.Name.Name).Str("nextVersion", nextVersion.String()).Msg("Tagging and pushing changes")
	if err := bp.tagAndPushChanges(service.GitRepoFilePath, *nextVersion); err != nil {
		return err
	}

	switch {
	case serviceConfiguration.Npm != nil:
		if err := bp.npmServiceProcessor.BuildAndDeployNpmService(ctx, service, nextVersion); err != nil {
			return err
		}
	case serviceConfiguration.OpenAPI != nil:
		log.Info().
			Str("service", service.Name.Name).
			Str("nextVersion", nextVersion.String()).
			Msg("Building and publishing OpenAPI npm client")

		if err := bp.openAPIProcessor.BuildAndDeployOpenAPIClient(ctx, service, nextVersion); err != nil {
			return fmt.Errorf("failed to build and deploy OpenAPI npm client: %w", err)
		}
	}

	return nil
}

func (bp *backgroundProcessor) shouldProcess(ctx context.Context, service *model.Service) (bool, error) {
	repo, err := git.PlainOpen(service.GitRepoFilePath)
	if err != nil {
		return false, fmt.Errorf("failed to open repo: %w", err)
	}

	// Pull from remote
	wt, err := repo.Worktree()
	if err != nil {
		return false, fmt.Errorf("failed to get worktree: %w", err)
	}

	err = wt.Pull(&git.PullOptions{
		RemoteName: defaultOrigin,
		Progress:   nil,
		Force:      true,
		Auth:       bp.sshAuth,
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return false, fmt.Errorf("failed to pull: %w", err)
	}

	// Get current HEAD
	ref, err := repo.Head()
	if err != nil {
		return false, fmt.Errorf("failed to get HEAD: %w", err)
	}

	c, err := repo.CommitObject(ref.Hash())
	if err != nil {
		return false, fmt.Errorf("failed to get commit object: %w", err)
	}

	tags, err := repo.Tags()
	if err != nil {
		return false, fmt.Errorf("failed to get tags: %w", err)
	}

	foundSemver := false
	err = tags.ForEach(func(ref *plumbing.Reference) error {
		// Try to resolve annotated tag objects
		tagObj, err := repo.TagObject(ref.Hash())
		var targetHash plumbing.Hash
		if err == nil {
			// Annotated tag -> resolve to its target
			targetHash = tagObj.Target
		} else {
			// Lightweight tag -> points directly to commit
			targetHash = ref.Hash()
		}

		// Compare tag's target to current commit
		if targetHash != c.Hash {
			return nil
		}

		tagName := ref.Name().Short()

		// Attempt to parse as semver
		if _, err := semver.NewVersion(tagName); err == nil {
			foundSemver = true
			return storer.ErrStop // stop iteration early
		}

		return nil
	})
	if err != nil && err != storer.ErrStop {
		return false, fmt.Errorf("error iterating tags: %w", err)
	}

	return !foundSemver, nil
}

func (bp *backgroundProcessor) commitChanges(repoPath string, version *semver.Version) error {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return fmt.Errorf("failed to open repo: %w", err)
	}

	workTree, err := repo.Worktree()
	if err != nil {
		return err
	}

	if err = workTree.AddGlob("*"); err != nil {
		return err
	}
	_, err = workTree.Commit(fmt.Sprintf(ciCommitMsgFormat, version.String()), &git.CommitOptions{
		Author: &object.Signature{
			Name:  bp.ciCommmitAuthor.Name,
			Email: bp.ciCommmitAuthor.Email,
			When:  time.Now(),
		},
	})
	if err != nil {
		return err
	}

	if err := repo.Push(&git.PushOptions{
		Auth:       bp.sshAuth,
		RemoteName: bp.gitRepoOrigin,
	}); err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to push commit: %w", err)
	}
	return nil
}

func (bp *backgroundProcessor) tagAndPushChanges(repoPath string, version semver.Version) error {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return fmt.Errorf("failed to open repo: %w", err)
	}

	head, err := repo.Head()
	if err != nil {
		return fmt.Errorf("failed to get HEAD: %w", err)
	}

	_, err = repo.CreateTag(version.String(), head.Hash(), &git.CreateTagOptions{
		Tagger: &object.Signature{
			Name:  bp.ciCommmitAuthor.Name,
			Email: bp.ciCommmitAuthor.Email,
			When:  time.Now(),
		},
		Message: fmt.Sprintf("Release %s", version.String()),
	})
	if err != nil {
		return fmt.Errorf("failed to create tag: %w", err)
	}

	if err := repo.Push(&git.PushOptions{
		Auth:       bp.sshAuth,
		RemoteName: bp.gitRepoOrigin,
		RefSpecs:   []config.RefSpec{"refs/tags/*:refs/tags/*"},
	}); err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to push tags: %w", err)
	}
	return nil
}
