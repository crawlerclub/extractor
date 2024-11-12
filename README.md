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
go env -w GOPRIVATE=github.com/crawlerclub/extractor
git config --global url."git@github.com:".insteadOf "https://github.com/"

go install github.com/crawlerclub/extractor/rabbitcrawler@latest
```
