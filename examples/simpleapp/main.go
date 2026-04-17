// Package main is the entrypoint for the gokux microservice.
// It demonstrates how to use the gokux framework to build a
// Kubernetes-ready service with minimal boilerplate.
package main

import (
	"flag"

	"github.com/dengliu/gokux"
)

func main() {
	// Parse CLI flags for config file paths (-f can be repeated).
	var configFiles []string
	flag.Func("f", "config file path (can be repeated; later files override earlier)", func(s string) error {
		configFiles = append(configFiles, s)
		return nil
	})
	flag.Parse()

	app := gokux.New(
		gokux.WithConfigFiles(configFiles...),
	)
	if err := app.Run(); err != nil {
		panic(err)
	}
}
