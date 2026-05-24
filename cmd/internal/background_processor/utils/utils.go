package utils

import (
	"bytes"
	"fmt"
	"os"
	"text/template"
)

func GenerateFileFromTemplate(
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
