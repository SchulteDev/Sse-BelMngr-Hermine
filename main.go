package main

import (
	"github.com/SchulteDev/Sse-BelMngr-Hermine/cli"
	"os"
)

func main() {
	if err := cli.Command.Execute(); err != nil {
		os.Exit(1)
	}
}
