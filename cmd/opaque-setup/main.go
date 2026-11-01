package main

import (
	"log"
	"os"

	"arkive/core/opaque"
)

func main() {
	setup, err := opaque.GenerateSetup()
	if err != nil {
		log.Fatal(err)
	}
	defer setup.Close()

	if _, err := os.Stdout.Write(setup.Bytes()); err != nil {
		log.Fatal(err)
	}
	if _, err := os.Stdout.Write([]byte{'\n'}); err != nil {
		log.Fatal(err)
	}
}
