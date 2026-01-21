package typescriptclient

import _ "embed"

//go:embed package.json.tmpl
var PackageJSONTemplate string

//go:embed openapi-ts.config.ts.tmpl
var OpenapiConfigTemplate string

//go:embed .prettierrc.json
var PrettierrcContent string

//go:embed Dockerfile.typescript-client
var DockerfileContent string
