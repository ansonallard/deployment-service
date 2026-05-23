package service_version

import _ "embed"

var (
	//go:embed version.txt
	ServiceVersion string
)
