package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sync"

	"extractor"

	"github.com/cheggaaa/pb/v3"
)

var (
	configFile = flag.String("config", "", "Path to the config JSON file")
	urlFile    = flag.String("urls", "", "File containing URLs to process, one per line")
	workers    = flag.Int("workers", 2, "Number of concurrent workers")
	outputFile = flag.String("output", "output.json", "Path to output JSON file")
	mode       = flag.String("mode", "auto", "Mode: auto, browser or static")
)

type Result struct {
	URL   string                     `json:"url"`
	Error string                     `json:"error,omitempty"`
	Data  extractor.ExtractionResult `json:"data,omitempty"`
}

func main() {
	flag.Parse()

	if *configFile == "" || *urlFile == "" {
		log.Fatal("Both config file and URL file are required")
	}

	config, err := loadConfig(*configFile)
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	urls, err := loadURLs(*urlFile)
	if err != nil {
		log.Fatalf("Error loading URLs: %v", err)
	}

	bar := pb.Full.Start(len(urls))
	defer bar.Finish()

	results := make(chan Result)
	var wg sync.WaitGroup

	urlChan := make(chan string, *workers)

	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go worker(config, urlChan, results, &wg)
	}

	done := make(chan bool)
	go collectResults(results, done, bar)

	for _, url := range urls {
		urlChan <- url
	}
	close(urlChan)

	wg.Wait()
	close(results)
	<-done

	log.Printf("Processing completed. Results saved to %s", *outputFile)
}

func loadConfig(path string) (extractor.ExtractorConfig, error) {
	var config extractor.ExtractorConfig
	data, err := os.ReadFile(path)
	if err != nil {
		return config, fmt.Errorf("reading config file: %w", err)
	}

	if err := json.Unmarshal(data, &config); err != nil {
		return config, fmt.Errorf("parsing config JSON: %w", err)
	}
	return config, nil
}

func loadURLs(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening URL file: %w", err)
	}
	defer file.Close()

	var urls []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		url := scanner.Text()
		if url != "" {
			urls = append(urls, url)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading URL file: %w", err)
	}
	return urls, nil
}

func worker(config extractor.ExtractorConfig, urls <-chan string, results chan<- Result, wg *sync.WaitGroup) {
	defer wg.Done()

	var e extractor.Extractor
	switch *mode {
	case "static":
		e = extractor.NewStaticExtractor(config)
	case "browser":
		e = extractor.NewBrowserExtractor(config)
	default:
		e = extractor.NewExtractor(config)
	}

	for url := range urls {
		result, err := e.Extract(url)
		if err != nil {
			results <- Result{
				URL:   url,
				Error: err.Error(),
			}
			continue
		}

		results <- Result{
			URL:  url,
			Data: *result,
		}
	}
}

func collectResults(results <-chan Result, done chan<- bool, bar *pb.ProgressBar) {
	file, err := os.OpenFile(*outputFile, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Error opening output file: %v", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetEscapeHTML(false)

	for result := range results {
		if err := encoder.Encode(result); err != nil {
			log.Printf("Error saving result for URL %s: %v", result.URL, err)
		}

		bar.Increment()
	}
	done <- true
}
