// syn is a short alias for the synapse CLI.
//
// Both `syn` and `synapse` are produced from the same internal/cli package
// and behave identically. This shim exists so users can type `syn` without
// having to set up shell aliases.
package main

import "github.com/swiftj/synapse/internal/cli"

func main() {
	cli.RunMain()
}
