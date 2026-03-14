package otel

import "embed"

// TemplateFS exposes the OTel collector manifest templates used by config generation.
//
//go:embed templates/*.tmpl
var TemplateFS embed.FS
