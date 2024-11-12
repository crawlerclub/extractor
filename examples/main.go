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
	configFile = flag.String("config", "", "Path to the config JSON file")
	url        = flag.String("url", "", "URL to extract data from")
	mode       = flag.String("mode", "auto", "Mode: auto, browser or static")
)

func main() {
	flag.Parse()

	if *configFile == "" {
		log.Fatal("config file is required")
	}

	configData, err := os.ReadFile(*configFile)
	if err != nil {
		log.Fatalf("Error reading config file: %v", err)
	}

	var config extractor.ExtractorConfig
	if err := json.Unmarshal(configData, &config); err != nil {
		log.Fatalf("Error parsing config JSON: %v", err)
	}

	if *url == "" {
		*url = config.ExampleURL
	}
	if *url == "" {
		log.Fatal("url is required and must be provided via flag or in the config")
	}

	var worker extractor.Extractor
	switch *mode {
	case "static":
		worker = extractor.NewStaticExtractor(config)
	case "browser":
		worker = extractor.NewBrowserExtractor(config)
	default:
		worker = extractor.NewExtractor(config)
	}
	result, err := worker.Extract(*url)

	if err != nil {
		log.Fatalf("Error extracting data: %v", err)
	}

	if len(result.Errors) > 0 {
		log.Println("Extraction completed with errors:")
		for _, err := range result.Errors {
			log.Printf("Field '%s' from %s: %s", err.Field, err.URL, err.Message)
		}
	}

	jsonData, err := goutil.JSONMarshalIndent(result.SchemaResults, "", "  ")
	if err != nil {
		log.Fatalf("Error converting results to JSON: %v", err)
	}

	fmt.Println(string(jsonData))
}
