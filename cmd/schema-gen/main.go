package main

import (
	"log"

	"github.com/erickivel/oksg/pkg/core/schema"
)

func main() {
	if err := schema.GenerateSchemas("../../.."); err != nil {
		log.Fatal(err)
	}
}
