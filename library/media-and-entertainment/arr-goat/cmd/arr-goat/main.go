package main

import (
	"fmt"
	"os"

	"github.com/mvanhorn/printing-press-library/library/media-and-entertainment/arr-goat/internal/cli"
	"github.com/mvanhorn/printing-press-library/library/media-and-entertainment/arr-goat/internal/config"
)

func main() {
	cfg, err := config.Load(config.DefaultConfigPath())
	if err != nil {
		fmt.Fprintln(os.Stderr, "arr-goat:", err)
		os.Exit(1)
	}
	if err := cli.NewRoot(cfg).Execute(); err != nil {
		os.Exit(1)
	}
}
