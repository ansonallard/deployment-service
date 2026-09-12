package rustclient

import _ "embed"

//go:embed Dockerfile.rust-client
var DockerfileTemplate string
