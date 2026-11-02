package main

import (
	"os"
	"typonamer/command"
)

func main() { os.Exit(command.Run(os.Args[1:], os.Stdout, os.Stderr)) }
