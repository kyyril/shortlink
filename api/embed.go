package api

import _ "embed"

// OpenAPISpec holds the raw OpenAPI 3.0 YAML specification embedded at build time.
//
//go:embed openapi.yaml
var OpenAPISpec []byte
