package npmservice

import _ "embed"

//go:embed Dockerfile.backend
var BackendDockerfile string

//go:embed Dockerfile.frontend
var FrontendDockerfile string

//go:embed frontend_nginx.conf
var FrontendNginxConfig string
