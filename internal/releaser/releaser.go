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
	"path/filepath"

	"github.com/Masterminds/semver/v3"
	"github.com/docker/docker/api/types/build"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/client"
	"github.com/rs/zerolog"
)

type DockerReleaser interface {
	BuildImage(ctx context.Context, serviceName, repositoryPath, dockerfilePath string, version *semver.Version) error
	PushImage(ctx context.Context, serviceName string, version *semver.Version) error
}

type DockerAuth struct {
	Username            string
	PersonalAccessToken string
	ServerAddress       string
}

type dockerReleaser struct {
	dockerclient   *client.Client
	artifactPrefix string
	registryAuth   *DockerAuth
}

type DockerReleaserConfig struct {
	DockerClient   *client.Client
	ArtifactPrefix string
	RegistryAuth   *DockerAuth
}

func NewDockerReleaser(config DockerReleaserConfig) (*dockerReleaser, error) {
	if config.DockerClient == nil {
		return nil, fmt.Errorf("dockerClient not provided")
	}
	if config.ArtifactPrefix == "" {
		return nil, fmt.Errorf("artifactPrefix not provided")
	}
	if config.RegistryAuth == nil {
		return nil, fmt.Errorf("registryAuth not provided")
	}
	return &dockerReleaser{
		dockerclient:   config.DockerClient,
		artifactPrefix: config.ArtifactPrefix,
		registryAuth:   config.RegistryAuth,
	}, nil
}

// BuildImage builds the Docker image using the official Docker SDK.
func (r *dockerReleaser) BuildImage(ctx context.Context, imageName, repositoryPath, dockerfilePath string, version *semver.Version) error {
	// Create build context tarball
	buildCtx, err := createTar(repositoryPath)
	if err != nil {
		return fmt.Errorf("failed to create build context: %w", err)
	}
	defer buildCtx.Close()

	localTag := fmt.Sprintf("%s:%s", imageName, version.String())

	// Run docker build
	resp, err := r.dockerclient.ImageBuild(ctx, buildCtx, build.ImageBuildOptions{
		Tags:        []string{localTag, r.createArtifactTag(imageName, version)},
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

// createTar creates a tar stream of the given directory (for Docker build context).
func createTar(dir string) (io.ReadCloser, error) {
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

	// Note: This code works, but it is failing to push the image to the artifact registry. Need to debug it's configuration (nginx)

	remoteImageTag := r.createArtifactTag(serviceName, version)
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

func (r *dockerReleaser) createArtifactTag(serviceName string, version *semver.Version) string {
	return fmt.Sprintf("%s/%s:%s", r.artifactPrefix, serviceName, version.String())
}
