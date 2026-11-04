// Command coderagent is the bhs-coderagent orchestrator. The service loop lands in F3.6
// (PLAN.md); until then it only reports its version.
package main

import (
	"flag"
	"fmt"
	"os"
)

// version is set at build time: -ldflags "-X main.version=…".
var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	fmt.Fprintln(os.Stderr, "coderagent: service loop not implemented yet (PLAN.md F3.6)")
	os.Exit(2)
}
