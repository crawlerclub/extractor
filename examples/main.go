package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"extractor"

	"zliu.org/goutil"
)

var (
	schemaFile = flag.String("schema", "", "Path to the schema JSON file")
	url        = flag.String("url", "", "URL to extract data from")
)

func main() {
	flag.Parse()

	if *schemaFile == "" {
		log.Fatal("schema file is required")
	}

	schemaData, err := os.ReadFile(*schemaFile)
	if err != nil {
		log.Fatalf("Error reading schema file: %v", err)
	}

	var schema extractor.Schema
	if err := json.Unmarshal(schemaData, &schema); err != nil {
		log.Fatalf("Error parsing schema JSON: %v", err)
	}

	if *url == "" {
		*url = schema.ExampleURL
	}
	if *url == "" {
		log.Fatal("url is required and must be provided via flag or in the schema")
	}

	extractor := extractor.NewExtractor(schema)
	result, err := extractor.Extract(*url)
	if err != nil {
		log.Fatalf("Error extracting data: %v", err)
	}

	if len(result.Errors) > 0 {
		log.Println("Extraction completed with errors:")
		for _, err := range result.Errors {
			log.Printf("Field '%s' from %s: %s", err.Field, err.URL, err.Message)
		}
	}

	jsonData, err := goutil.JSONMarshalIndent(result.Items, "", "  ")
	if err != nil {
		log.Fatalf("Error converting results to JSON: %v", err)
	}

	fmt.Println(string(jsonData))
}
