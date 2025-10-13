package releaser

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Masterminds/semver/v3"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

type DockerReleaser interface {
	BuildImage(ctx context.Context, imageName, repositoryPath, dockerfilePath string, version *semver.Version) error
}

type dockerReleaser struct {
	dockerclient *client.Client
}

type DockerReleaserConfig struct {
	DockerClient *client.Client
}

func NewDockerReleaser(config DockerReleaserConfig) *dockerReleaser {
	return &dockerReleaser{
		dockerclient: config.DockerClient,
	}
}

// BuildImage builds the Docker image using the official Docker SDK.
func (r *dockerReleaser) BuildImage(ctx context.Context, imageName, repositoryPath, dockerfilePath string, version *semver.Version) error {
	tag := fmt.Sprintf("%s:%s", imageName, version.String())

	// Create build context tarball
	buildCtx, err := createTar(repositoryPath)
	if err != nil {
		return fmt.Errorf("failed to create build context: %w", err)
	}
	defer buildCtx.Close()

	// Run docker build
	resp, err := r.dockerclient.ImageBuild(ctx, buildCtx, types.ImageBuildOptions{
		Tags:        []string{tag},
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

// // PushImage pushes the built Docker image to the registry.
// func (r *Releaser) PushImage() error {
// 	ctx := context.Background()

// 	// Create go-sdk client
// 	dockerClient, err := client.New(ctx)
// 	if err != nil {
// 		return fmt.Errorf("failed to create docker client: %w", err)
// 	}
// 	defer dockerClient.Close()

// 	tag := fmt.Sprintf("%s:%s", r.ImageName, r.NextVersion)

// 	// Push the image
// 	// _, err = dockerClient.ImagePush(ctx, tag, types.ImagePushOptions{
// 	// 	RegistryAuth: registry.AuthConfig{
// 	// 		Username: "your-username",
// 	// 		Password: "your-password",
// 	// 	},
// 	// })
// 	if err != nil {
// 		return fmt.Errorf("image push failed: %w", err)
// 	}

// 	return nil
// }

// UpdateVersionFiles updates version information in relevant files.
// func (r *Releaser) UpdateVersionFiles() error {
// 	// Update package.json version
// 	packageJSONPath := filepath.Join(r.RepositoryPath, "package.json")
// 	// Logic to update package.json version...

// 	// Optionally update openapi.yaml version
// 	if r.UpdateOpenAPI {
// 		openAPIPath := filepath.Join(r.RepositoryPath, "src", "public", "openapi.yaml")
// 		// Logic to update openapi.yaml version...
// 	}

// 	return nil
// }

// // CommitAndTag commits the changes and tags the release.
// func (r *Releaser) CommitAndTag() error {
// 	// Logic to commit changes and tag the release...
// 	return nil
// }

// BuildAndRelease orchestrates the entire release process.
// func (r *dockerReleaser) BuildAndRelease() error {
// 	if err := r.BuildImage(); err != nil {
// 		return fmt.Errorf("failed to build image: %w", err)
// 	}

// 	// if err := r.PushImage(); err != nil {
// 	// 	return fmt.Errorf("failed to push image: %w", err)
// 	// }

// 	// if err := r.UpdateVersionFiles(); err != nil {
// 	// 	return fmt.Errorf("failed to update version files: %w", err)
// 	// }

// 	// if err := r.CommitAndTag(); err != nil {
// 	// 	return fmt.Errorf("failed to commit and tag: %w", err)
// 	// }

// 	return nil
// }
