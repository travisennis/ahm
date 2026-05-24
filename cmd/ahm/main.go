package main

import (
	"os"

	"github.com/travisennis/ahm/internal/ahm"
)

func main() {
	os.Exit(ahm.Main(os.Args[1:], os.Stdout, os.Stderr))
}
