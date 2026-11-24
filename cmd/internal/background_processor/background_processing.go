package backgroundprocessor

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/ansonallard/deployment-service/cmd/internal/background_processor/dockerbuild"
	"github.com/ansonallard/deployment-service/cmd/internal/background_processor/dockercompose"
	"github.com/ansonallard/deployment-service/cmd/internal/background_processor/goservice"
	"github.com/ansonallard/deployment-service/cmd/internal/background_processor/npm"
	"github.com/ansonallard/deployment-service/cmd/internal/background_processor/openapi"
	"github.com/ansonallard/deployment-service/cmd/internal/background_processor/utils"
	"github.com/ansonallard/deployment-service/cmd/internal/metrics"
	"github.com/ansonallard/deployment-service/cmd/internal/model"
	"github.com/ansonallard/deployment-service/cmd/internal/version"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("deployment-service.background")

const (
	ciCommitMsgFormat = "ci: Release version %s"
	defaultOrigin     = "origin"
)

type BackgroundProcesseror interface {
	ProcessService(ctx context.Context, service *model.Service) error
}

type BackgroundProcessorConfig struct {
	Versioner              version.Versioner
	SSHKeyPath             string
	GitRepoOrigin          string
	CiCommitAuthor         *utils.CiCommitAuthor
	NpmServiceProcessor    npm.NPMServiceProcessor
	OpenAPIProcessor       openapi.OpenAPIProcessor
	GoServiceProcessor     goservice.GoServiceProcessor
	DockerComposeProcessor dockercompose.DockerComposeProcessor
	DockerBuildProcessor   dockerbuild.DockerBuildProcessor
	IsDev                  bool
	SelfServiceName        string
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
	if config.GoServiceProcessor == nil {
		return nil, fmt.Errorf("goServiceProcessor not provided")
	}
	if config.DockerComposeProcessor == nil {
		return nil, fmt.Errorf("dockerComposeProcessor not provided")
	}
	if config.DockerBuildProcessor == nil {
		return nil, fmt.Errorf("dockerBuildProcessor not provided")
	}

	return &backgroundProcessor{
			versioner:              config.Versioner,
			gitRepoOrigin:          config.GitRepoOrigin,
			sshAuth:                sshAuth,
			ciCommmitAuthor:        config.CiCommitAuthor,
			npmServiceProcessor:    config.NpmServiceProcessor,
			openAPIProcessor:       config.OpenAPIProcessor,
			goServiceProcessor:     config.GoServiceProcessor,
			dockerComposeProcessor: config.DockerComposeProcessor,
			dockerBuildProcessor:   config.DockerBuildProcessor,
			isDevMode:              config.IsDev,
			selfServiceName:        config.SelfServiceName,
		},
		nil
}

type backgroundProcessor struct {
	versioner              version.Versioner
	sshAuth                *ssh.PublicKeys
	gitRepoOrigin          string
	ciCommmitAuthor        *utils.CiCommitAuthor
	npmServiceProcessor    npm.NPMServiceProcessor
	openAPIProcessor       openapi.OpenAPIProcessor
	goServiceProcessor     goservice.GoServiceProcessor
	dockerComposeProcessor dockercompose.DockerComposeProcessor
	dockerBuildProcessor   dockerbuild.DockerBuildProcessor
	isDevMode              bool
	selfDeployMu           sync.RWMutex
	selfServiceName        string
}

func (bp *backgroundProcessor) ProcessService(ctx context.Context, service *model.Service) error {
	log := zerolog.Ctx(ctx)

	start := time.Now().UTC()
	var buildType string
	defer func() {
		if buildType != "" {
			metrics.BuildDuration.WithLabelValues(service.Name.Name, buildType).Observe(float64(time.Since(start).Milliseconds()))
		}
	}()

	_, checkSpan := tracer.Start(ctx, "background.has_new_commit",
		trace.WithAttributes(attribute.String("service.name", service.Name.Name)),
	)
	hasNewCommit, err := bp.hasNewCommit(ctx, service)
	if err != nil {
		checkSpan.RecordError(err)
		checkSpan.SetStatus(codes.Error, err.Error())
		checkSpan.End()
		return err
	}
	checkSpan.End()

	if !hasNewCommit {
		if service.Configuration.DockerCompose != nil && service.Configuration.DockerCompose.RefreshImages {
			unlock := bp.acquireDeployLock(ctx, service)
			defer unlock()
			return bp.dockerComposeProcessor.RefreshDockerComposeApplication(ctx, service)
		}
		return nil
	}

	unlock := bp.acquireDeployLock(ctx, service)
	defer unlock()

	calcCtx, calcSpan := tracer.Start(ctx, "background.calculate_next_version",
		trace.WithAttributes(attribute.String("service.name", service.Name.Name)),
	)
	nextVersion, err := bp.versioner.CalculateNextVersion(calcCtx, service.GitRepoFilePath)
	if err != nil {
		calcSpan.RecordError(err)
		calcSpan.SetStatus(codes.Error, err.Error())
		calcSpan.End()
		return err
	}
	calcSpan.End()
	log.Info().Interface("semver", nextVersion).Str("nextVersion", nextVersion.String()).Msg("Next version")

	serviceConfiguration := service.Configuration
	_, setVerSpan := tracer.Start(ctx, "background.set_version",
		trace.WithAttributes(attribute.String("service.name", service.Name.Name)),
	)
	var changedPaths []string
	switch {
	case serviceConfiguration.Npm != nil:
		log.Info().Str("service", service.Name.Name).Str("nextVersion", nextVersion.String()).Msg("Npm Service")
		changedPaths, err = bp.npmServiceProcessor.SetPackageJsonVersion(service, nextVersion)
	case serviceConfiguration.OpenAPI != nil:
		log.Info().Str("service", service.Name.Name).Str("nextVersion", nextVersion.String()).Msg("OpenAPI Service")
		changedPaths, err = bp.openAPIProcessor.SetOpenApiYamlVersion(service, nextVersion)
	case serviceConfiguration.Go != nil:
		log.Info().Str("service", service.Name.Name).Str("nextVersion", nextVersion.String()).Msg("Go Service")
		changedPaths, err = bp.goServiceProcessor.SetVersionFile(service, nextVersion)
	case serviceConfiguration.DockerCompose != nil:
		log.Info().Str("service", service.Name.Name).Str("nextVersion", nextVersion.String()).Msg("Docker Compose Service")
	case serviceConfiguration.DockerBuild != nil:
		log.Info().Str("service", service.Name.Name).Str("nextVersion", nextVersion.String()).Msg("Docker Build Service")
	}

	if err != nil {
		setVerSpan.RecordError(err)
		setVerSpan.SetStatus(codes.Error, err.Error())
		setVerSpan.End()
		return err
	}

	setVerSpan.End()

	if !bp.isDevMode {
		log.Info().Str("service", service.Name.Name).Str("nextVersion", nextVersion.String()).
			Strs("changedPaths", changedPaths).Msg("Commiting changes")
		if err := bp.commitChanges(ctx, service.GitRepoFilePath, nextVersion, changedPaths); err != nil {
			return err
		}

		log.Info().Str("service", service.Name.Name).Str("nextVersion", nextVersion.String()).Msg("Tagging and pushing changes")
		if err := bp.tagAndPushChanges(ctx, service.GitRepoFilePath, *nextVersion); err != nil {
			return err
		}
	}

	buildCtx, buildSpan := tracer.Start(ctx, "background.build",
		trace.WithAttributes(attribute.String("service.name", service.Name.Name)),
	)

	buildType, err = bp.runBuild(buildCtx, buildSpan, log, service, nextVersion)
	buildSpan.End()

	if err != nil {
		if buildType != "" {
			metrics.BuildAttempts.WithLabelValues(service.Name.Name, buildType, "failure").Inc()
		}
		return err
	}
	if buildType != "" {
		metrics.BuildAttempts.WithLabelValues(service.Name.Name, buildType, "success").Inc()
	}

	return nil
}

func (bp *backgroundProcessor) runBuild(
	buildCtx context.Context,
	buildSpan trace.Span,
	log *zerolog.Logger,
	service *model.Service,
	nextVersion *semver.Version,
) (buildType string, err error) {
	start := time.Now()
	defer func() {
		if buildType != "" {
			metrics.BuildDuration.WithLabelValues(service.Name.Name, buildType).Observe(time.Since(start).Seconds())
		}
	}()

	cfg := service.Configuration
	switch {
	case cfg.Npm != nil && cfg.Npm.Service != nil:
		buildType = "npm"
		if err = bp.npmServiceProcessor.BuildNpmService(buildCtx, service, nextVersion); err != nil {
			buildSpan.RecordError(err)
			buildSpan.SetStatus(codes.Error, err.Error())
			return buildType, err
		}

	case cfg.OpenAPI != nil:
		buildType = "openapi"
		log.Info().
			Str("service", service.Name.Name).
			Str("nextVersion", nextVersion.String()).
			Msg("Building and publishing OpenAPI npm client")

		if err = bp.openAPIProcessor.BuildAndDeployOpenAPIClient(buildCtx, service, nextVersion); err != nil {
			buildSpan.RecordError(err)
			buildSpan.SetStatus(codes.Error, err.Error())
			return buildType, fmt.Errorf("failed to build and deploy OpenAPI npm client: %w", err)
		}

	case cfg.Go != nil && cfg.Go.Service != nil:
		buildType = "go"
		log.Info().
			Str("service", service.Name.Name).
			Str("nextVersion", nextVersion.String()).
			Msg("Building Go service")

		if err = bp.goServiceProcessor.BuildGoService(buildCtx, service, nextVersion); err != nil {
			buildSpan.RecordError(err)
			buildSpan.SetStatus(codes.Error, err.Error())
			return buildType, fmt.Errorf("failed to build Go service: %w", err)
		}

	case cfg.DockerCompose != nil:
		buildType = "docker_compose"
		log.Info().
			Str("service", service.Name.Name).
			Str("nextVersion", nextVersion.String()).
			Msg("Deploying Docker Compose application")

		if err = bp.dockerComposeProcessor.DeployDockerComposeApplication(buildCtx, service, nextVersion); err != nil {
			buildSpan.RecordError(err)
			buildSpan.SetStatus(codes.Error, err.Error())
			return buildType, fmt.Errorf("failed to deploy Docker Compose application: %w", err)
		}

	case cfg.DockerBuild != nil:
		buildType = "docker_build"
		log.Info().
			Str("service", service.Name.Name).
			Str("nextVersion", nextVersion.String()).
			Msg("Building and pushing Docker image")

		if err = bp.dockerBuildProcessor.BuildAndPushDockerImage(buildCtx, service, nextVersion); err != nil {
			buildSpan.RecordError(err)
			buildSpan.SetStatus(codes.Error, err.Error())
			return buildType, fmt.Errorf("failed to build and push Docker image: %w", err)
		}

	default:
		log.Error().
			Str("service", service.Name.Name).
			Str("nextVersion", nextVersion.String()).
			Msg("Service type not supported")
		// original behavior: logs only, does not return an error
	}

	return buildType, nil
}

func (bp *backgroundProcessor) hasNewCommit(ctx context.Context, service *model.Service) (bool, error) {
	ctx, span := tracer.Start(ctx, "background.pull_and_check",
		trace.WithAttributes(attribute.String("service.name", service.Name.Name)),
	)
	defer span.End()

	repo, err := git.PlainOpen(service.GitRepoFilePath)
	if err != nil {
		return false, fmt.Errorf("failed to open repo: %w", err)
	}

	if err := bp.syncToRemote(ctx, repo, bp.preservedPaths()); err != nil {
		return false, err
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

// preservedPaths returns repo-relative files that must survive the hard reset
// in syncToRemote (everything else untracked is wiped). Match whatever
// path/envFile the env writer uses for this service.
func (bp *backgroundProcessor) preservedPaths() []string {
	return []string{".env"}
}

// syncToRemote makes the local checkout exactly match origin/<current branch>,
// discarding any local modifications (e.g. generated Dockerfiles from prior builds).
// Files listed in preserve (repo-relative) are snapshotted before the reset and
// restored right after, because go-git's HardReset also deletes untracked files.
func (bp *backgroundProcessor) syncToRemote(ctx context.Context, repo *git.Repository, preserve []string) (err error) {
	head, err := repo.Head()
	if err != nil {
		return fmt.Errorf("failed to get HEAD: %w", err)
	}
	if !head.Name().IsBranch() {
		return fmt.Errorf("HEAD is not on a branch: %s", head.Name())
	}
	branch := head.Name().Short()

	err = repo.FetchContext(ctx, &git.FetchOptions{
		RemoteName: defaultOrigin,
		RefSpecs:   []config.RefSpec{"+refs/heads/*:refs/remotes/origin/*"},
		Tags:       git.AllTags,
		Force:      true,
		Auth:       bp.sshAuth,
	})
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return fmt.Errorf("failed to fetch: %w", err)
	}

	remoteRef, err := repo.Reference(plumbing.NewRemoteReferenceName(defaultOrigin, branch), true)
	if err != nil {
		return fmt.Errorf("failed to resolve %s/%s: %w", defaultOrigin, branch, err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	root := wt.Filesystem.Root()
	saved, err := snapshotFiles(root, preserve)
	if err != nil {
		return fmt.Errorf("failed to snapshot preserved files: %w", err)
	}
	// Restore even if the reset fails partway through
	defer func() {
		if rerr := restoreFiles(root, saved); rerr != nil {
			err = errors.Join(err, fmt.Errorf("failed to restore preserved files: %w", rerr))
		}
	}()

	if err := wt.Reset(&git.ResetOptions{Commit: remoteRef.Hash(), Mode: git.HardReset}); err != nil {
		return fmt.Errorf("failed to hard reset to %s: %w", remoteRef.Hash(), err)
	}
	return nil
}

func (bp *backgroundProcessor) commitChanges(ctx context.Context, repoPath string, version *semver.Version, changedPaths []string) error {
	ctx, span := tracer.Start(ctx, "background.commit",
		trace.WithAttributes(
			attribute.String("repo_path", repoPath),
			attribute.String("version", version.String()),
		),
	)
	defer span.End()

	repoRoot, err := filepath.Abs(repoPath)
	if err != nil {
		return err
	}
	repo, err := git.PlainOpen(repoRoot)
	if err != nil {
		return fmt.Errorf("failed to open repo: %w", err)
	}

	workTree, err := repo.Worktree()
	if err != nil {
		return err
	}

	expected := make(map[string]struct{}, len(changedPaths))
	for _, p := range changedPaths {
		rel, err := toRepoRelative(repoRoot, p)
		if err != nil {
			return err
		}
		if _, err := workTree.Add(rel); err != nil {
			return fmt.Errorf("failed to stage %s: %w", rel, err)
		}
		expected[rel] = struct{}{}
	}

	// Guard: only the files the providers reported may be staged.
	status, err := workTree.Status()
	if err != nil {
		return fmt.Errorf("failed to read status: %w", err)
	}
	for p, s := range status {
		if s.Staging == git.Unmodified || s.Staging == git.Untracked {
			continue
		}
		if _, ok := expected[p]; !ok {
			return fmt.Errorf("refusing to commit: unexpected staged change to %s", p)
		}
	}

	_, err = workTree.Commit(fmt.Sprintf(ciCommitMsgFormat, version.String()), &git.CommitOptions{
		Author: &object.Signature{
			Name:  bp.ciCommmitAuthor.Name,
			Email: bp.ciCommmitAuthor.Email,
			When:  time.Now(),
		},
		AllowEmptyCommits: true,
	})
	if err != nil {
		return err
	}

	if err := repo.Push(&git.PushOptions{
		Auth:       bp.sshAuth,
		RemoteName: bp.gitRepoOrigin,
	}); err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return fmt.Errorf("failed to push commit: %w", err)
	}
	return nil
}

func toRepoRelative(root, p string) (string, error) {
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("path %q is not absolute", p)
	}
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q is outside repo %q", p, root)
	}
	return filepath.ToSlash(rel), nil
}

func (bp *backgroundProcessor) tagAndPushChanges(ctx context.Context, repoPath string, version semver.Version) error {
	ctx, span := tracer.Start(ctx, "background.tag",
		trace.WithAttributes(
			attribute.String("repo_path", repoPath),
			attribute.String("version", version.String()),
		),
	)
	defer span.End()

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

func (bp *backgroundProcessor) acquireDeployLock(ctx context.Context, service *model.Service) func() {
	log := zerolog.Ctx(ctx)

	if bp.isSelf(service) {
		bp.selfDeployMu.Lock()
		log.Debug().Msg("Acquired RW lock")
		return func() {
			bp.selfDeployMu.Unlock()
			log.Debug().Msg("Released RW lock")
		}
	}
	log.Debug().Msg("Acquired R lock")
	bp.selfDeployMu.RLock()
	return func() {
		bp.selfDeployMu.RUnlock()
		log.Debug().Msg("Released R lock")
	}
}

func (bp *backgroundProcessor) isSelf(service *model.Service) bool {
	if bp.selfServiceName == "" {
		return false
	}
	return service.Name.Name == bp.selfServiceName
}
