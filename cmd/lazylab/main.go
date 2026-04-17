package main

import (
	"fmt"
	"io"
	"os"

	"github.com/integrii/flaggy"

	"github.com/niklod/lazylab/internal/cli"
)

func main() {
	flaggy.SetName("lazylab")
	flaggy.SetDescription("Terminal UI for GitLab")
	flaggy.DefaultParser.ShowVersionWithVersionFlag = false

	versionCmd := flaggy.NewSubcommand("version")
	versionCmd.Description = "Print version and exit"
	flaggy.AttachSubcommand(versionCmd, 1)

	runCmd := flaggy.NewSubcommand("run")
	runCmd.Description = "Launch the terminal UI"
	flaggy.AttachSubcommand(runCmd, 1)

	flaggy.Parse()

	switch {
	case versionCmd.Used:
		exitOnErr(os.Stderr, cli.Version(os.Stdout))
	case runCmd.Used:
		exitOnErr(os.Stderr, cli.Run(os.Stdout))
	default:
		flaggy.ShowHelp("")
		os.Exit(1)
	}
}

func exitOnErr(stderr io.Writer, err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(stderr, err)
	os.Exit(1)
}
