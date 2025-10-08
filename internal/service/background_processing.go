package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/ansonallard/deployment-service/internal/model"
	"github.com/ansonallard/deployment-service/internal/version"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/rs/zerolog"
)

const (
	packageJSONFilePath   = "package.json"
	packageJSONVersionKey = "version"
	ciCommitMsgFormat     = "ci: Release version %s"
)

type BackgroundProcesseror interface {
	ProcessService(ctx context.Context, service *model.Service) error
}

type BackgroundProcessorConfig struct {
	Versioner      *version.Versioner
	SSHKeyPath     string
	GitRepoOrigin  string
	CiCommitAuthor CiCommitAuthor
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
	return &backgroundProcessor{
			versioner:       *config.Versioner,
			gitRepoOrigin:   config.GitRepoOrigin,
			sshAuth:         sshAuth,
			ciCommmitAuthor: config.CiCommitAuthor,
		},
		nil
}

type backgroundProcessor struct {
	versioner       version.Versioner
	sshAuth         *ssh.PublicKeys
	gitRepoOrigin   string
	ciCommmitAuthor CiCommitAuthor
}

func (bp *backgroundProcessor) ProcessService(ctx context.Context, service *model.Service) error {
	log := zerolog.Ctx(ctx)
	nextVersion, err := bp.versioner.CalculateNextVersion(ctx, service.GitRepoFilePath)
	if err != nil {
		return err
	}
	log.Info().Interface("semver", nextVersion).Str("nextVersion", nextVersion.String()).Msg("Next version")

	serviceConfiguration := service.Configuration
	switch {
	case serviceConfiguration.Npm != nil:
		log.Info().Str("nextVersion", nextVersion.String()).Msg("Npm Service")
		if err := bp.setPackageJsonVersion(service, nextVersion); err != nil {
			return err
		}
	}
	if err := bp.commitChanges(service.GitRepoFilePath, nextVersion); err != nil {
		return err
	}

	if err := bp.tagAndPushChanges(service.GitRepoFilePath, *nextVersion); err != nil {
		return err
	}

	return nil
}

func (bp *backgroundProcessor) setPackageJsonVersion(service *model.Service, version *semver.Version) error {
	if _, err := os.Stat(service.GitRepoFilePath); err != nil {
		return err
	}

	packageJsonFilePath := bp.getPackageJsonPath(service.GitRepoFilePath)

	fileBytes, err := os.ReadFile(packageJsonFilePath)
	if err != nil {
		return err
	}
	var packageJson map[string]any
	if err := json.Unmarshal(fileBytes, &packageJson); err != nil {
		return err
	}
	packageJson[packageJSONVersionKey] = version.String()
	packageJsonBytes, err := json.Marshal(packageJson)
	if err != nil {
		return err
	}
	if err := os.WriteFile(packageJsonFilePath, packageJsonBytes, 644); err != nil {
		return nil
	}
	return nil
}

func (bp *backgroundProcessor) getPackageJsonPath(gitRepoFilePath string) string {
	return path.Join(gitRepoFilePath, packageJSONFilePath)
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
