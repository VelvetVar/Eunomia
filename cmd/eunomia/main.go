package main

import (
	"eunomia/internal/eunomia"
	"os"
)

func main() { os.Exit(eunomia.Main(os.Args[1:], os.Stdout, os.Stderr)) }
