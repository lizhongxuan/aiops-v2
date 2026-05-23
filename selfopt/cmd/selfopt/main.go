package main

import (
	"os"

	"aiops-v2/selfopt"
)

func main() {
	os.Exit(selfopt.Main(os.Args[1:], os.Stdout, os.Stderr))
}
