package main

import (
	"fmt"
	"os"

	"github.com/ivmm/exam-run/cmd/skctl/cmd"
)

func main() {
	if err := cmd.NewRoot().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
