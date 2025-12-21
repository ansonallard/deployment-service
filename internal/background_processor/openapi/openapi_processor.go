package openapi

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/ansonallard/deployment-service/internal/model"
	"github.com/ansonallard/deployment-service/internal/releaser"
	goclient "github.com/ansonallard/deployment-service/internal/templates/go_client"
	typescriptclient "github.com/ansonallard/deployment-service/internal/templates/typescript_client"
	yaml "github.com/oasdiff/yaml3"
	"github.com/rs/zerolog"
)

const (
	npmrcSecretKey = "npmrc"
	gitTeaPATKey   = "gitea_token"
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
}

type goClientTemplateData struct {
	ModulePath      string
	Version         string
	PackageName     string
	OpenAPIFileName string
	OutputPath      string
	ServiceName     string
	RegistryUrl     string
}

type TypescriptClientConfig struct {
	NpmrcPath    string // Path to .npmrc file for npm authentication
	PackageScope string // e.g., "ansonallard" for @ansonallard/package-name
}

type GoClientConfig struct {
	Token          string // Authentication token for Gitea
	ModuleBasePath string // e.g., "gitea.yourcompany.com/clients"
}

type OpenAPIProcessorConfig struct {
	TypescriptClientConfig *TypescriptClientConfig
	GoClientConfig         *GoClientConfig
	DockerReleaser         releaser.DockerReleaser
	RegistryUrl            string
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
	if config.RegistryUrl == "" {
		return nil, fmt.Errorf("RegistryURL not provided")
	}
	if config.GoClientConfig.Token == "" {
		return nil, fmt.Errorf("GoClientConfig.Token not provided")
	}
	if config.GoClientConfig.ModuleBasePath == "" {
		return nil, fmt.Errorf("GoClientConfig.ModuleBasePath not provided")
	}
	return &openAPIProcessor{
		typescriptClientConfig: config.TypescriptClientConfig,
		goClientConfig:         config.GoClientConfig,
		dockerReleaser:         config.DockerReleaser,
		registryUrl:            config.RegistryUrl,
	}, nil
}

type openAPIProcessor struct {
	typescriptClientConfig *TypescriptClientConfig
	goClientConfig         *GoClientConfig
	dockerReleaser         releaser.DockerReleaser
	registryUrl            string
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
	// Validate OpenAPI configuration
	if service.Configuration.OpenAPI == nil || service.Configuration.OpenAPI.OpenAPI == nil {
		return fmt.Errorf("openAPI configuration is nil")
	}

	// Build TypeScript client if configured
	if service.Configuration.OpenAPI.OpenAPI.TypescriptClient != nil {
		if err := op.buildAndDeployTypescriptClient(ctx, service, nextVersion); err != nil {
			return fmt.Errorf("failed to build TypeScript client: %w", err)
		}
	}

	// Build Go client if configured
	if service.Configuration.OpenAPI.OpenAPI.GoClient != nil {
		if err := op.buildAndDeployGoClient(ctx, service, nextVersion); err != nil {
			return fmt.Errorf("failed to build Go client: %w", err)
		}
	}

	return nil
}

func (op *openAPIProcessor) buildAndDeployTypescriptClient(
	ctx context.Context,
	service *model.Service,
	nextVersion *semver.Version,
) error {
	log := zerolog.Ctx(ctx)

	log.Info().
		Str("service", service.Name.Name).
		Str("version", nextVersion.String()).
		Msg("Starting TypeScript OpenAPI client generation and publication")

	// Create build directory
	buildDir, err := op.createOpenAPIClientBuildDir(service, nextVersion, "typescript")
	if err != nil {
		return fmt.Errorf("failed to create build directory: %w", err)
	}

	// Image name for cleanup
	imageName := fmt.Sprintf("%s-openapi-typescript-client-builder", service.Name.Name)

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
	if err := op.generateTypescriptClientConfigFiles(buildDir, service, nextVersion); err != nil {
		return fmt.Errorf("failed to generate config files: %w", err)
	}

	// Copy OpenAPI spec
	if err := op.copyOpenAPISpec(service, buildDir); err != nil {
		return fmt.Errorf("failed to copy OpenAPI spec: %w", err)
	}

	// Build and publish using Docker
	if err := op.buildAndPublishTypescriptDockerClient(ctx, buildDir, imageName, nextVersion); err != nil {
		return fmt.Errorf("failed to build and publish client: %w", err)
	}

	log.Info().
		Str("service", service.Name.Name).
		Str("version", nextVersion.String()).
		Msg("Successfully published TypeScript OpenAPI npm client")

	return nil
}

func (op *openAPIProcessor) buildAndDeployGoClient(
	ctx context.Context,
	service *model.Service,
	nextVersion *semver.Version,
) error {
	log := zerolog.Ctx(ctx)

	log.Info().
		Str("service", service.Name.Name).
		Str("version", nextVersion.String()).
		Msg("Starting Go OpenAPI client generation and publication")

	// Create build directory
	buildDir, err := op.createOpenAPIClientBuildDir(service, nextVersion, "go")
	if err != nil {
		return fmt.Errorf("failed to create build directory: %w", err)
	}

	defer func() {
		// Cleanup build directory
		if err := os.RemoveAll(buildDir); err != nil {
			log.Error().Err(err).Str("buildDir", buildDir).Msg("Failed to cleanup build directory")
		}
	}()

	// Generate Go client using Docker
	imageName := fmt.Sprintf("%s-openapi-go-client-builder", service.Name.Name)

	defer func() {
		// Cleanup Docker image
		if err := op.dockerReleaser.RemoveImage(
			ctx, op.generateOpenAPIDockerFullyQualifiedImageName(imageName, nextVersion),
		); err != nil {
			log.Error().Err(err).Str("imageTag", imageName).Msg("Failed to remove Docker image")
		}
	}()

	// Generate configuration files for Go client
	if err := op.generateGoClientConfigFiles(buildDir, service, nextVersion); err != nil {
		return fmt.Errorf("failed to generate Go client config files: %w", err)
	}

	// Copy OpenAPI spec
	if err := op.copyOpenAPISpec(service, buildDir); err != nil {
		return fmt.Errorf("failed to copy OpenAPI spec: %w", err)
	}

	// Build Go client using Docker
	if err := op.buildGoClientDocker(ctx, buildDir, imageName, nextVersion); err != nil {
		return fmt.Errorf("failed to build Go client: %w", err)
	}

	log.Info().
		Str("service", service.Name.Name).
		Str("version", nextVersion.String()).
		Msg("Successfully published Go OpenAPI client to Gitea")

	return nil
}

func (op *openAPIProcessor) createOpenAPIClientBuildDir(
	service *model.Service,
	version *semver.Version,
	clientType string,
) (string, error) {
	buildID := generateBuildID()
	buildDir := filepath.Join(
		os.TempDir(),
		fmt.Sprintf("openapi-client-build-%s-%s-%s", clientType, service.Name.Name, buildID),
	)

	if err := os.MkdirAll(buildDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create build directory: %w", err)
	}

	return buildDir, nil
}

func (op *openAPIProcessor) generateTypescriptClientConfigFiles(
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

func (op *openAPIProcessor) generateGoClientConfigFiles(
	buildDir string,
	service *model.Service,
	version *semver.Version,
) error {
	packageName := op.generateGoClientName(service)

	modulePath := op.getGoClientModuleName(service)
	outputPath := "./lib"

	templateData := goClientTemplateData{
		ModulePath:      modulePath,
		Version:         op.generateGoClientVersion(version),
		PackageName:     packageName,
		OpenAPIFileName: filepath.Base(service.Configuration.OpenAPI.OpenAPI.YamlFile),
		OutputPath:      outputPath,
		ServiceName:     service.Name.Name,
		RegistryUrl:     op.registryUrl,
	}

	// Generate oapi-codegen config
	if err := op.generateFileFromTemplate(
		filepath.Join(buildDir, "config.yaml"),
		goclient.OapiConfigTemplate,
		templateData,
	); err != nil {
		return fmt.Errorf("failed to generate config.yaml: %w", err)
	}

	// Generate go.mod
	if err := op.generateFileFromTemplate(
		filepath.Join(buildDir, "go.mod"),
		goclient.GoModTemplate,
		templateData,
	); err != nil {
		return fmt.Errorf("failed to generate go.mod: %w", err)
	}

	// Generate Dockerfile
	if err := op.generateFileFromTemplate(
		filepath.Join(buildDir, "Dockerfile"),
		goclient.DockerfileTemplate,
		templateData,
	); err != nil {
		return fmt.Errorf("failed to generate Dockerfile: %w", err)
	}

	return nil
}

func (op *openAPIProcessor) getGoClientModuleName(service *model.Service) string {
	return fmt.Sprintf("%s/%s", op.goClientConfig.ModuleBasePath, op.generateGoClientName(service))
}

func (op *openAPIProcessor) generateGoClientName(service *model.Service) string {
	var clientName string
	switch {
	case service.Configuration.OpenAPI.OpenAPI.GoClient != nil &&
		service.Configuration.OpenAPI.OpenAPI.GoClient.Name.Name != "":
		clientName = service.Configuration.OpenAPI.OpenAPI.GoClient.Name.Name
	default:
		clientName = fmt.Sprintf("%s-go-client", service.Name.Name)
	}
	return strings.ReplaceAll(clientName, "-", "_")
}

func (op *openAPIProcessor) generateGoClientVersion(version *semver.Version) string {
	return fmt.Sprintf("v%s", version.String())
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

func (op *openAPIProcessor) buildAndPublishTypescriptDockerClient(
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

func (op *openAPIProcessor) buildGoClientDocker(
	ctx context.Context,
	buildDir string,
	imageName string,
	version *semver.Version,
) error {
	return op.dockerReleaser.BuildImageWithSecrets(
		ctx,
		buildDir,
		"Dockerfile",
		[]string{op.generateOpenAPIDockerFullyQualifiedImageName(imageName, version)},
		map[string][]byte{
			gitTeaPATKey: []byte(op.goClientConfig.Token),
		},
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
