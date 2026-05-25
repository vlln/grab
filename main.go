package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/vlln/grab/cmd/grab"
	"github.com/vlln/grab/internal/config"
)

func main() {
	if err := grab.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		var misconfig *config.MisconfigError
		if errors.As(err, &misconfig) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}