package goclient

import _ "embed"

//go:embed go.mod.tmpl
var GoModTemplate string

//go:embed oapi_config.yaml.tmpl
var OapiConfigTemplate string

//go:embed Dockerfile.go-client
var DockerfileTemplate string

//go:embed embed.go.tmpl
var EmbedGoTemplate string
