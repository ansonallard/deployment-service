package releaser

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"

	"github.com/Masterminds/semver/v3"
	"github.com/docker/docker/api/types/build"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/client"
	"github.com/rs/zerolog"
)

type DockerReleaser interface {
	BuildImage(ctx context.Context, repositoryPath, dockerfilePath string, tags []string) error
	BuildImageWithSecrets(ctx context.Context, repositoryPath, dockerfilePath string, tags []string, secrets map[string][]byte) error
	PushImage(ctx context.Context, serviceName string, version *semver.Version) error
	RemoveImage(ctx context.Context, tag string) error
	FullyQualifiedImageTag(imageName string, version *semver.Version) string
	CreateArtifactTag(serviceName string, version *semver.Version) string
}

type DockerAuth struct {
	Username            string
	PersonalAccessToken string
	ServerAddress       string
}

type dockerReleaser struct {
	dockerclient    *client.Client
	artifactPrefix  string
	registryAuth    *DockerAuth
	pathToDockerCLI string
}

type DockerReleaserConfig struct {
	DockerClient    *client.Client
	ArtifactPrefix  string
	RegistryAuth    *DockerAuth
	PathToDockerCLI string
}

func NewDockerReleaser(config DockerReleaserConfig) (DockerReleaser, error) {
	if config.DockerClient == nil {
		return nil, fmt.Errorf("dockerClient not provided")
	}
	if config.ArtifactPrefix == "" {
		return nil, fmt.Errorf("artifactPrefix not provided")
	}
	if config.RegistryAuth == nil {
		return nil, fmt.Errorf("registryAuth not provided")
	}
	if config.PathToDockerCLI == "" {
		return nil, fmt.Errorf("pathToDockerCLi not provided")
	}

	return &dockerReleaser{
		dockerclient:    config.DockerClient,
		artifactPrefix:  config.ArtifactPrefix,
		registryAuth:    config.RegistryAuth,
		pathToDockerCLI: config.PathToDockerCLI,
	}, nil
}

// BuildImage builds the Docker image using the official Docker SDK.
func (r *dockerReleaser) BuildImage(ctx context.Context, repositoryPath, dockerfilePath string, tags []string) error {
	// Create build context tarball
	buildCtx, err := CreateTar(repositoryPath)
	if err != nil {
		return fmt.Errorf("failed to create build context: %w", err)
	}
	defer buildCtx.Close()

	// Run docker build
	resp, err := r.dockerclient.ImageBuild(ctx, buildCtx, build.ImageBuildOptions{
		Tags:        tags,
		Dockerfile:  dockerfilePath,
		Remove:      true,
		ForceRemove: true,
	})
	if err != nil {
		return fmt.Errorf("image build failed: %w", err)
	}
	defer resp.Body.Close()

	// Stream build logs to stdout
	_, err = io.Copy(os.Stdout, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read build output: %w", err)
	}

	return nil
}

const (
	dockerBuildCommand = "build"
)

// The Moby SDK doesn't support builds with secrets.
// Docker doesn't expose buildkit directly either, which means
// we can't use that SDK. For now, use a subprocess to build an
// image with secrets. Once this is supported in the Go SDK,
// switch over to the SDK.
func (r *dockerReleaser) BuildImageWithSecrets(
	ctx context.Context,
	repositoryPath,
	dockerfilePath string,
	tags []string,
	secrets map[string][]byte,
) error {
	log := zerolog.Ctx(ctx)

	log.Info().
		Str("repositoryPath", repositoryPath).
		Str("dockerfile", dockerfilePath).
		Strs("tags", tags).
		Int("secretCount", len(secrets)).
		Msg("Building image with secrets using docker CLI")

	// Create temporary directory for secret files with unique build ID
	tempDir, err := os.MkdirTemp("", "docker-secrets-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Write secrets to temporary files
	secretFiles := make(map[string]string)
	for id, content := range secrets {
		secretPath := filepath.Join(tempDir, id)
		if err := os.WriteFile(secretPath, content, 0600); err != nil {
			return fmt.Errorf("failed to write secret %s: %w", id, err)
		}
		secretFiles[id] = secretPath
		log.Debug().
			Str("secretId", id).
			Str("path", secretPath).
			Msg("Created temporary secret file")
	}

	// Build docker command arguments
	args := []string{dockerBuildCommand}

	// Add secret arguments
	for id, path := range secretFiles {
		args = append(args, "--secret", fmt.Sprintf("id=%s,src=%s", id, path))
	}

	// Add tags
	for _, tag := range tags {
		args = append(args, "-t", tag)
	}

	// Add dockerfile and context
	fullyQualifiedDockerfilePath := path.Join(repositoryPath, dockerfilePath)
	args = append(args, "-f", fullyQualifiedDockerfilePath, repositoryPath)

	log.Debug().
		Strs("args", args).
		Msg("Executing docker build command")

	// Create command
	cmd := exec.CommandContext(ctx, r.pathToDockerCLI, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Execute the build
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker build command failed: %w", err)
	}

	log.Info().
		Strs("tags", tags).
		Msg("Image build with secrets completed successfully")

	return nil
}

// CreateTar creates a tar stream of the given directory (exported for use in other packages)
func CreateTar(dir string) (io.ReadCloser, error) {
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	err := filepath.Walk(dir, func(file string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if fi.Mode().IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(dir, file)
		if err != nil {
			return err
		}

		f, err := os.Open(file)
		if err != nil {
			return err
		}
		defer f.Close()

		hdr, err := tar.FileInfoHeader(fi, relPath)
		if err != nil {
			return err
		}
		hdr.Name = relPath

		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}

		_, err = io.Copy(tw, f)
		return err
	})

	if err != nil {
		tw.Close()
		return nil, err
	}

	tw.Close()
	return io.NopCloser(bytes.NewReader(buf.Bytes())), nil
}

// PushImage pushes the built Docker image to the registry.
func (r *dockerReleaser) PushImage(ctx context.Context, serviceName string, version *semver.Version) error {
	log := zerolog.Ctx(ctx)

	remoteImageTag := r.CreateArtifactTag(serviceName, version)
	log.Info().
		Str("service", serviceName).
		Str("nextVersion", version.String()).
		Str("remoteImageTag", remoteImageTag).
		Msg("Pushing image")

	authConfig := registry.AuthConfig{
		Username:      r.registryAuth.Username,
		Password:      r.registryAuth.PersonalAccessToken,
		ServerAddress: r.registryAuth.ServerAddress,
	}

	authJSON, err := json.Marshal(authConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal auth config: %w", err)
	}

	// Push the image
	response, err := r.dockerclient.ImagePush(ctx, remoteImageTag, image.PushOptions{
		RegistryAuth: base64.StdEncoding.EncodeToString(authJSON),
	})
	if err != nil {
		return fmt.Errorf("image push failed: %w", err)
	}
	defer response.Close()

	// Stream response line by line
	scanner := bufio.NewScanner(response)
	for scanner.Scan() {
		line := scanner.Text()
		log.Info().
			Str("service", serviceName).
			Str("version", version.String()).
			Str("imagePushOutput", line).
			Msg("Image push progress")
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed reading push output: %w", err)
	}

	log.Info().
		Str("service", serviceName).
		Str("version", version.String()).
		Msg("Image push completed")

	return nil
}

// RemoveImage removes a Docker image from the local system
func (r *dockerReleaser) RemoveImage(ctx context.Context, tag string) error {
	log := zerolog.Ctx(ctx)

	log.Info().
		Str("tag", tag).
		Msg("Removing Docker image")

	_, err := r.dockerclient.ImageRemove(ctx, tag, image.RemoveOptions{
		Force:         true,
		PruneChildren: true,
	})
	if err != nil {
		return fmt.Errorf("failed to remove image: %w", err)
	}

	log.Info().
		Str("tag", tag).
		Msg("Docker image removed successfully")

	return nil
}

func (r *dockerReleaser) FullyQualifiedImageTag(imageName string, version *semver.Version) string {
	return fmt.Sprintf("%s:%s", imageName, version.String())
}

func (r *dockerReleaser) CreateArtifactTag(serviceName string, version *semver.Version) string {
	return fmt.Sprintf("%s/%s", r.artifactPrefix, r.FullyQualifiedImageTag(serviceName, version))
}
