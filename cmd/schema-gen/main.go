package main

import (
	"log"

	"github.com/erickivel/karthos/pkg/core/schema"
)

func main() {
	if err := schema.GenerateSchemas("../../.."); err != nil {
		log.Fatal(err)
	}
}
