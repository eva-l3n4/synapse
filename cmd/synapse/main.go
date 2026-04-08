// Synapse - The shared nervous system for Vibe Coders and their Agents.
//
// A lightweight, local-first, Git-backed issue tracker designed to serve
// as persistent "long-term memory" for AI agents.
//
// All command logic lives in internal/cli. This binary is a thin shim so
// the `synapse` and `syn` binaries always behave identically.
package main

import "github.com/swiftj/synapse/internal/cli"

// version mirrors cli.Version. The post-commit Git hook scans this line
// (`const version = "X.Y.Z"`) to auto-bump on every commit that touches Go
// files. Keep both in sync.
const version = "1.0.10"

func main() {
	_ = version // referenced by the version-bump hook; cli.Version is the truth
	cli.RunMain()
}
