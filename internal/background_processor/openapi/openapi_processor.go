package openapi

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"text/template"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/ansonallard/deployment-service/internal/model"
	"github.com/ansonallard/deployment-service/internal/releaser"
	typescriptclient "github.com/ansonallard/deployment-service/internal/templates/typescript_client"
	yaml "github.com/oasdiff/yaml3"
	"github.com/rs/zerolog"
)

const (
	npmrcSecretKey = "npmrc"
)

type OpenAPIProcessor interface {
	SetOpenApiYamlVersion(service *model.Service, version *semver.Version) error
	BuildAndDeployOpenAPIClient(
		ctx context.Context,
		service *model.Service,
		nextVersion *semver.Version,
	) error
}

type openAPIClientTemplateData struct {
	PackageName     string
	Version         string
	Description     string
	OpenAPIFileName string
	OutputPath      string
	ServiceName     string
	RegistryURL     string
}

type TypescriptClientConfig struct {
	NpmrcPath    string // Path to .npmrc file for npm authentication
	PackageScope string // e.g., "ansonallard" for @ansonallard/package-name
	RegistryURL  string // e.g., "https://npm-registry.example.com"
}

type OpenAPIProcessorConfig struct {
	TypescriptClientConfig *TypescriptClientConfig
	DockerReleaser         releaser.DockerReleaser
}

func NewOpenAPIProcessor(config OpenAPIProcessorConfig) (OpenAPIProcessor, error) {
	if config.DockerReleaser == nil {
		return nil, fmt.Errorf("dockerReleaser not provided")
	}
	if config.TypescriptClientConfig == nil {
		return nil, fmt.Errorf("TypescriptClientConfig not provided")
	}
	if config.TypescriptClientConfig.NpmrcPath == "" {
		return nil, fmt.Errorf("TypescriptClientConfig.NpmrcPath not provided")
	}
	if config.TypescriptClientConfig.PackageScope == "" {
		return nil, fmt.Errorf("TypescriptClientConfig.PackageScope not provided")
	}
	if config.TypescriptClientConfig.RegistryURL == "" {
		return nil, fmt.Errorf("TypescriptClientConfig.RegistryURL not provided")
	}
	return &openAPIProcessor{
		typescriptClientConfig: config.TypescriptClientConfig,
		dockerReleaser:         config.DockerReleaser,
	}, nil

}

type openAPIProcessor struct {
	typescriptClientConfig *TypescriptClientConfig
	dockerReleaser         releaser.DockerReleaser
}

func (op *openAPIProcessor) SetOpenApiYamlVersion(service *model.Service, version *semver.Version) error {
	if _, err := os.Stat(service.GitRepoFilePath); err != nil {
		return err
	}

	openApiYamlFilePath := path.Join(service.GitRepoFilePath, service.Configuration.OpenAPI.OpenAPI.YamlFile)
	fileBytes, err := os.ReadFile(openApiYamlFilePath)
	if err != nil {
		return err
	}

	// Parse YAML into a Node to preserve order and formatting
	var node yaml.Node
	if err := yaml.Unmarshal(fileBytes, &node); err != nil {
		return err
	}

	// Navigate to info.version and update it
	if err := updateVersion(&node, version); err != nil {
		return err
	}

	// Marshal back to YAML
	openApiYamlBytes, err := yaml.Marshal(&node)
	if err != nil {
		return err
	}

	if err := os.WriteFile(openApiYamlFilePath, openApiYamlBytes, 0644); err != nil {
		return err
	}

	return nil
}

func updateVersion(node *yaml.Node, version *semver.Version) error {
	// The root node is a Document node, we need the mapping inside it
	if node.Kind != yaml.DocumentNode || len(node.Content) == 0 {
		return fmt.Errorf("invalid yaml structure")
	}

	root := node.Content[0]
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("root is not a mapping")
	}

	// Find the "info" key
	for i := 0; i < len(root.Content); i += 2 {
		if root.Content[i].Value == "info" {
			infoNode := root.Content[i+1]
			if infoNode.Kind != yaml.MappingNode {
				return fmt.Errorf("info is not a mapping")
			}

			// Find the "version" key within info
			for j := 0; j < len(infoNode.Content); j += 2 {
				if infoNode.Content[j].Value == "version" {
					infoNode.Content[j+1].Value = version.String()
					return nil
				}
			}
			return fmt.Errorf("version key not found in info")
		}
	}
	return fmt.Errorf("info key not found")
}

func (op *openAPIProcessor) BuildAndDeployOpenAPIClient(
	ctx context.Context,
	service *model.Service,
	nextVersion *semver.Version,
) error {
	log := zerolog.Ctx(ctx)

	// Validate OpenAPI configuration
	if service.Configuration.OpenAPI == nil || service.Configuration.OpenAPI.OpenAPI == nil {
		return fmt.Errorf("openAPI configuration is nil")
	}

	log.Info().
		Str("service", service.Name.Name).
		Str("version", nextVersion.String()).
		Msg("Starting OpenAPI client generation and publication")

	// Create build directory
	buildDir, err := op.createOpenAPIClientBuildDir(service, nextVersion)
	if err != nil {
		return fmt.Errorf("failed to create build directory: %w", err)
	}

	// Image name for cleanup
	imageName := fmt.Sprintf("%s-openapi-client-builder", service.Name.Name)

	defer func() {
		// Cleanup build directory
		if err := os.RemoveAll(buildDir); err != nil {
			log.Error().Err(err).Str("buildDir", buildDir).Msg("Failed to cleanup build directory")
		}

		// Cleanup Docker image
		if err := op.dockerReleaser.RemoveImage(
			ctx, op.generateOpenAPIDockerFullyQualifiedImageName(imageName, nextVersion),
		); err != nil {
			log.Error().Err(err).Str("imageTag", imageName).Msg("Failed to remove Docker image")
		}
	}()

	// Generate configuration files
	if err := op.generateClientConfigFiles(buildDir, service, nextVersion); err != nil {
		return fmt.Errorf("failed to generate config files: %w", err)
	}

	// Copy OpenAPI spec
	if err := op.copyOpenAPISpec(service, buildDir); err != nil {
		return fmt.Errorf("failed to copy OpenAPI spec: %w", err)
	}

	// Build and publish using Docker
	if err := op.buildAndPublishDockerClient(ctx, buildDir, imageName, nextVersion); err != nil {
		return fmt.Errorf("failed to build and publish client: %w", err)
	}

	log.Info().
		Str("service", service.Name.Name).
		Str("version", nextVersion.String()).
		Msg("Successfully published OpenAPI npm client")

	return nil
}

func (op *openAPIProcessor) createOpenAPIClientBuildDir(
	service *model.Service,
	version *semver.Version,
) (string, error) {
	buildID := generateBuildID()
	buildDir := filepath.Join(
		os.TempDir(),
		fmt.Sprintf("openapi-client-build-%s-%s", service.Name.Name, buildID),
	)

	if err := os.MkdirAll(buildDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create build directory: %w", err)
	}

	return buildDir, nil
}

func (op *openAPIProcessor) generateClientConfigFiles(
	buildDir string,
	service *model.Service,
	version *semver.Version,
) error {
	var clientPackageName string
	switch {
	case service.Configuration.OpenAPI.OpenAPI.TypescriptClient != nil:
		clientPackageName = service.Configuration.OpenAPI.OpenAPI.TypescriptClient.Name.Name
	default:
		clientPackageName = fmt.Sprintf("%s-typescript-client", service.Name.Name)
	}
	packageName := fmt.Sprintf("@%s/%s", op.typescriptClientConfig.PackageScope, clientPackageName)
	outputPath := fmt.Sprintf("./lib/%s", clientPackageName)

	templateData := openAPIClientTemplateData{
		PackageName:     packageName,
		Version:         version.String(),
		Description:     fmt.Sprintf("TypeScript SDK for %s", service.Name.Name),
		OpenAPIFileName: filepath.Base(service.Configuration.OpenAPI.OpenAPI.YamlFile),
		OutputPath:      outputPath,
		ServiceName:     service.Name.Name,
		RegistryURL:     op.typescriptClientConfig.RegistryURL,
	}

	// Generate package.json
	if err := op.generateFileFromTemplate(
		filepath.Join(buildDir, "package.json"),
		typescriptclient.PackageJSONTemplate,
		templateData,
	); err != nil {
		return fmt.Errorf("failed to generate package.json: %w", err)
	}

	// Generate openapi-ts.config.ts
	if err := op.generateFileFromTemplate(
		filepath.Join(buildDir, "openapi-ts.config.ts"),
		typescriptclient.OpenapiConfigTemplate,
		templateData,
	); err != nil {
		return fmt.Errorf("failed to generate openapi-ts.config.ts: %w", err)
	}

	// Write .prettierrc.json
	if err := os.WriteFile(
		filepath.Join(buildDir, ".prettierrc.json"),
		[]byte(typescriptclient.PrettierrcContent),
		0644,
	); err != nil {
		return fmt.Errorf("failed to write .prettierrc.json: %w", err)
	}

	// Write Dockerfile
	if err := os.WriteFile(
		filepath.Join(buildDir, "Dockerfile"),
		[]byte(typescriptclient.DockerfileContent),
		0644,
	); err != nil {
		return fmt.Errorf("failed to write Dockerfile: %w", err)
	}

	return nil
}

func (op *openAPIProcessor) generateFileFromTemplate(
	outputPath string,
	tmplContent string,
	data interface{},
) error {
	tmpl, err := template.New("").Parse(tmplContent)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	if err := os.WriteFile(outputPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

func (op *openAPIProcessor) copyOpenAPISpec(service *model.Service, buildDir string) error {
	sourcePath := filepath.Join(
		service.GitRepoFilePath,
		service.Configuration.OpenAPI.OpenAPI.YamlFile,
	)

	destPath := filepath.Join(
		buildDir,
		filepath.Base(service.Configuration.OpenAPI.OpenAPI.YamlFile),
	)

	return copyFile(sourcePath, destPath)
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	return nil
}

func (op *openAPIProcessor) buildAndPublishDockerClient(
	ctx context.Context,
	buildDir string,
	imageName string,
	version *semver.Version,
) error {
	secrets, err := op.generateDockerBuildSecrets()
	if err != nil {
		return err
	}
	return op.dockerReleaser.BuildImageWithSecrets(
		ctx,
		buildDir,
		"Dockerfile",
		[]string{op.generateOpenAPIDockerFullyQualifiedImageName(imageName, version)},
		secrets,
	)
}

func (op *openAPIProcessor) generateDockerBuildSecrets() (map[string][]byte, error) {
	npmrcBytes, err := os.ReadFile(op.typescriptClientConfig.NpmrcPath)
	if err != nil {
		return nil, err
	}
	return map[string][]byte{
		npmrcSecretKey: npmrcBytes,
	}, nil
}

func (op *openAPIProcessor) generateOpenAPIDockerFullyQualifiedImageName(imageName string, version *semver.Version) string {
	return op.dockerReleaser.FullyQualifiedImageTag(imageName, version)
}

func generateBuildID() string {
	return time.Now().UTC().Format(time.RFC3339)
}
