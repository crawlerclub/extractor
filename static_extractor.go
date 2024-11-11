package extractor

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/antchfx/htmlquery"
	"golang.org/x/net/html"
)

type StaticExtractor struct {
	Config ExtractorConfig
}

func NewStaticExtractor(config ExtractorConfig) *StaticExtractor {
	return &StaticExtractor{Config: config}
}

func (e *StaticExtractor) Extract(url string) (*ExtractionResult, error) {
	// Create HTTP client with reasonable timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Make HTTP request
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch URL: %v", err)
	}
	defer resp.Body.Close()

	// Read response body
	htmlContent, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	result := &ExtractionResult{
		SchemaResults: make(map[string]SchemaResult),
		Errors:        make([]ExtractionError, 0),
	}

	doc, err := htmlquery.Parse(strings.NewReader(string(htmlContent)))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %v", err)
	}

	// Extract items for each schema
	for _, schema := range e.Config.Schemas {
		schemaResult := SchemaResult{
			Schema: SchemaInfo{
				Name:       schema.Name,
				EntityType: schema.EntityType,
			},
			Items: make([]ExtractedItem, 0),
		}

		elements, err := htmlquery.QueryAll(doc, schema.Selector)
		if err != nil {
			result.Errors = append(result.Errors, ExtractionError{
				Field:   schema.Name,
				Message: fmt.Sprintf("failed to find elements with selector: %s", schema.Selector),
			})
			continue
		}

		for _, element := range elements {
			item, errs := e.extractItemWithSchema(element, schema, url)
			if len(errs) > 0 {
				result.Errors = append(result.Errors, errs...)
			}
			if item != nil {
				// extract external_id
				externalID, ok := extractExternalID(item)
				if !ok {
					continue
				}
				item["external_id"] = strings.ToUpper(externalID)
				delete(item, "_id")

				// extract external_time
				if externalTime, ok := extractExternalTime(item); ok {
					item["external_time"] = externalTime
					delete(item, "_time")
				} else {
					item["external_time"] = time.Now()
				}
				schemaResult.Items = append(schemaResult.Items, item)
			}
		}

		result.SchemaResults[schema.Name] = schemaResult
	}

	return result, nil
}

func (e *StaticExtractor) extractItemWithSchema(element *html.Node, schema Schema, url string) (ExtractedItem, []ExtractionError) {
	item := make(ExtractedItem)
	var errors []ExtractionError

	for _, field := range schema.Fields {
		value, err := e.extractField(element, field, url)
		if err != nil {
			errors = append(errors, ExtractionError{
				Field:   field.Name,
				Message: err.Error(),
			})
			continue
		}
		item[field.Name] = value
	}

	return item, errors
}

func (e *StaticExtractor) extractField(element *html.Node, field Field, url string) (interface{}, error) {
	if strings.HasPrefix(field.Name, "_id") {
		// extract nested id
		if field.Type == "nested" {
			nestedElement := element
			nestedItem := make(ExtractedItem)
			for _, nestedField := range field.Fields {
				nestedValue, err := e.extractField(nestedElement, nestedField, url)
				if err != nil {
					continue
				}
				nestedItem[nestedField.Name] = nestedValue
			}
			if len(nestedItem) > 0 {
				return nestedItem, nil
			}
		}

		// if nested id not found, extract id from url or element
		switch field.From {
		case FromURL:
			matches := regexp.MustCompile(field.Pattern).FindStringSubmatch(url)
			if len(matches) > 1 {
				return strings.Join(matches[1:], "/"), nil
			}
			return nil, fmt.Errorf("failed to extract id from URL using pattern: %s", field.Pattern)
		case FromElement:
			el := htmlquery.FindOne(element, field.Selector)
			if el == nil {
				return nil, fmt.Errorf("element not found for selector: %s", field.Selector)
			}
			text := htmlquery.InnerText(el)
			matches := regexp.MustCompile(field.Pattern).FindStringSubmatch(text)
			if len(matches) > 1 {
				return strings.Join(matches[1:], "/"), nil
			}
			return nil, fmt.Errorf("failed to extract id from element using pattern: %s", field.Pattern)
		default:
			return nil, fmt.Errorf("unsupported from: %s", field.From)
		}
	}

	if strings.HasPrefix(field.Name, "_time") {
		switch field.From {
		case FromURL:
			matches := regexp.MustCompile(field.Pattern).FindStringSubmatch(url)
			if len(matches) > 1 {
				return strings.Join(matches[1:], "/"), nil
			}
			return nil, fmt.Errorf("failed to extract time from URL using pattern: %s", field.Pattern)
		case FromElement:
			el := htmlquery.FindOne(element, field.Selector)
			if el == nil {
				return nil, fmt.Errorf("element not found for selector: %s", field.Selector)
			}
			text := htmlquery.InnerText(el)
			matches := regexp.MustCompile(field.Pattern).FindStringSubmatch(text)
			if len(matches) > 1 {
				return strings.Join(matches[1:], "/"), nil
			}
			return nil, fmt.Errorf("failed to extract time from element using pattern: %s", field.Pattern)
		default:
			return nil, fmt.Errorf("unsupported from: %s", field.From)
		}
	}

	switch field.Type {
	case "text":
		el := htmlquery.FindOne(element, field.Selector)
		if el == nil {
			return "", fmt.Errorf("element not found for selector: %s", field.Selector)
		}
		return strings.TrimSpace(htmlquery.InnerText(el)), nil

	case "attribute":
		el := htmlquery.FindOne(element, field.Selector)
		if el == nil {
			return "", fmt.Errorf("element not found for selector: %s", field.Selector)
		}
		for _, attr := range el.Attr {
			if attr.Key == field.Attribute {
				return attr.Val, nil
			}
		}
		return "", fmt.Errorf("attribute %s not found", field.Attribute)

	case "nested":
		nestedElement := htmlquery.FindOne(element, field.Selector)
		if nestedElement == nil {
			return nil, fmt.Errorf("nested element not found for selector: %s", field.Selector)
		}
		nestedItem := make(ExtractedItem)
		for _, nestedField := range field.Fields {
			nestedValue, err := e.extractField(nestedElement, nestedField, url)
			if err != nil {
				continue
			}
			nestedItem[nestedField.Name] = nestedValue
		}
		if len(nestedItem) > 0 {
			return nestedItem, nil
		}
		return nil, fmt.Errorf("all nested fields failed to extract")

	case "list":
		elements := htmlquery.Find(element, field.Selector)
		if len(elements) == 0 {
			return nil, fmt.Errorf("elements not found for selector: %s", field.Selector)
		}

		// Check for single text field case
		if len(field.Fields) == 1 && field.Fields[0].Type == "text" && field.Fields[0].Selector == "." {
			var items []string
			for _, el := range elements {
				value, err := e.extractField(el, field.Fields[0], url)
				if err != nil {
					continue
				}
				if str, ok := value.(string); ok {
					items = append(items, str)
				}
			}
			return items, nil
		}

		var items []map[string]interface{}
		for _, el := range elements {
			item := make(map[string]interface{})
			for _, subField := range field.Fields {
				value, err := e.extractField(el, subField, url)
				if err != nil {
					continue
				}
				item[subField.Name] = value
			}
			if len(item) > 0 {
				items = append(items, item)
			}
		}
		return items, nil

	default:
		return nil, fmt.Errorf("unsupported field type: %s", field.Type)
	}
}
