package main

import (
	"fmt"
	"os"

	"github.com/muxx/redmine-cli/internal/cli"
)

var version = "dev"

func main() {
	cmd := cli.New(version)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
