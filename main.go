package main

import (
	"os"

	"github.com/SchulteDev/Sse-BelMngr-Hermine/cli"
)

func main() {
	if err := cli.Command.Execute(); err != nil {
		os.Exit(1)
	}
}
