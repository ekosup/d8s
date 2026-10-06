// Command d8s is a terminal UI for Docker Engine and Docker Swarm.
package main

import (
	"fmt"
	"os"

	"github.com/ekosup/d8s/internal/version"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "d8s:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 && args[0] == "version" {
		fmt.Println(version.String())
		return nil
	}
	return fmt.Errorf("nothing to run yet; try `d8s version`")
}
