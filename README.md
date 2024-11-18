# rabbitcrawler

A powerful web scraping tool designed for extracting structured data from websites with configurable rules and multiple execution modes.

## Features

- Configurable JSON-based scraping rules
- Multiple extraction modes:
  - Static: Fast HTML parsing without JavaScript execution
  - Browser: Full browser emulation with JavaScript support
- Concurrent scraping with adjustable worker count

## Installation

```bash
go install github.com/crawlerclub/extractor/cmd/rabbitextract@latest
go install github.com/crawlerclub/extractor/cmd/rabbitcrawler@latest
```

## Using rabbitextract

rabbitextract is a command-line tool for extracting data from a single webpage using JSON configuration rules.

### Command Line Options

- `-config`: Path to the config JSON file (required)
- `-url`: URL to extract data from (optional if provided in config)
- `-mode`: Extraction mode (optional, defaults to "auto")
  - `auto`: Automatically choose between static and browser mode
  - `static`: Fast HTML parsing without JavaScript
  - `browser`: Full browser emulation with JavaScript support
- `-output`: Output file path (optional, defaults to stdout)

### Example Usage

1. Create a configuration file `config.json`:
```json
{
  "name": "example-scraper",
  "example_url": "https://example.com/page",
  "schemas": [
    {
      "name": "articles",
      "entity_type": "article",
      "selector": "//div[@class='article']",
      "fields": [
        {
          "name": "title",
          "type": "text",
          "selector": ".//h1"
        },
        {
          "name": "content",
          "type": "text",
          "selector": ".//div[@class='content']"
        }
      ]
    }
  ]
}
```

2. Run the extractor:
```bash
rabbitextract -config config.json -url "https://example.com/page" -output result.json
```

### Supported Field Types

- `text`: Extract text content from an element
- `attribute`: Extract specific attribute value from an element
- `nested`: Extract nested object with multiple fields
- `list`: Extract array of items

### Special Fields

- `_id`: Used to generate unique external_id for items
- `_time`: Used to set external_time for items
