package main

import (
	"os"

	"github.com/vedansh-5/graphcontext/pkg/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:], os.Stdout, os.Stderr))
}
