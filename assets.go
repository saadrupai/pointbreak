// Package pointbreak holds the embedded context templates shared by the CLI and the MCP server.
package pointbreak

import "embed"

// Templates contains templates/context/** (the all: prefix keeps the _service folder).
//
//go:embed all:templates
var Templates embed.FS
