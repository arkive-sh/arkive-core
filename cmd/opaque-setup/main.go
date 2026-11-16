package main

import (
	"log"
	"os"

	"arkive/core/opaque"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	setup, err := opaque.GenerateSetup()
	if err != nil {
		return err
	}
	defer setup.Close()

	if _, err := os.Stdout.Write(setup.Bytes()); err != nil {
		return err
	}
	if _, err := os.Stdout.Write([]byte{'\n'}); err != nil {
		return err
	}
	return nil
}
