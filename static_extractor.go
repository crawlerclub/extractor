package extractor

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/antchfx/htmlquery"
	"github.com/crawlerclub/httpcache"
	"golang.org/x/net/html"
)

type StaticExtractor struct {
	Config ExtractorConfig
}

func NewStaticExtractor(config ExtractorConfig) *StaticExtractor {
	return &StaticExtractor{Config: config}
}

func (e *StaticExtractor) ExtractWithoutCache(url string) (*ExtractionResult, error) {
	return e.extract(url, false)
}

func (e *StaticExtractor) Extract(url string) (*ExtractionResult, error) {
	return e.extract(url, true)
}

func (e *StaticExtractor) extract(url string, cache bool) (*ExtractionResult, error) {
	client := httpcache.GetClient()
	var htmlContent []byte
	var finalURL string
	var err error
	if cache {
		htmlContent, finalURL, err = client.GetWithFinalURL(url)
		if err != nil {
			return nil, fmt.Errorf("failed to read response body: %v", err)
		}
	} else {
		htmlContent, finalURL, err = client.FetchWithFinalURL(url)
		if err != nil {
			return nil, fmt.Errorf("failed to read response body: %v", err)
		}
	}

	needDelete := true
	result := &ExtractionResult{
		SchemaResults: make(map[string]SchemaResult),
		Errors:        make([]ExtractionError, 0),
		FinalURL:      finalURL,
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
			item, errs := e.extractItemWithSchema(element, schema, url, doc)
			if len(errs) > 0 {
				result.Errors = append(result.Errors, errs...)
			}
			if item != nil {
				needDelete = false
				// extract external_id
				if externalID, ok := extractExternalID(item); ok {
					item["external_id"] = strings.ToUpper(externalID)
					delete(item, "_id")
				}

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

	if needDelete {
		client.DeleteURL(url)
	}

	return result, nil
}

func (e *StaticExtractor) extractItemWithSchema(element *html.Node, schema Schema, url string, doc *html.Node) (ExtractedItem, []ExtractionError) {
	item := make(ExtractedItem)
	var errors []ExtractionError

	for _, field := range schema.Fields {
		value, err := e.extractField(element, field, url, doc)
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

func (e *StaticExtractor) evaluateCount(countXPath string, element *html.Node, doc *html.Node) (int, error) {
	if !isValidXPath(countXPath) {
		return 0, fmt.Errorf("invalid XPath expression: %s", countXPath)
	}

	if strings.HasPrefix(countXPath, "//") {
		nodes := htmlquery.Find(doc, countXPath)
		return len(nodes), nil
	} else {
		nodes := htmlquery.Find(element, countXPath)
		return len(nodes), nil
	}
}

func (e *StaticExtractor) extractField(element *html.Node, field Field, url string, doc *html.Node) (interface{}, error) {
	// Helper function to handle XPath queries
	queryElement := func(selector string, contextNode *html.Node) (*html.Node, error) {
		if strings.HasPrefix(selector, "//") {
			return htmlquery.FindOne(doc, selector), nil
		}
		return htmlquery.FindOne(contextNode, selector), nil
	}

	queryElements := func(selector string, contextNode *html.Node) ([]*html.Node, error) {
		if strings.HasPrefix(selector, "//") {
			return htmlquery.Find(doc, selector), nil
		}
		return htmlquery.Find(contextNode, selector), nil
	}

	// Helper function to process count expressions in selector
	processCountExpression := func(selector string) (string, error) {
		for strings.Contains(selector, "count(") {
			start := strings.Index(selector, "count(")
			if start == -1 {
				break
			}

			bracketCount := 1
			end := start + 6
			for end < len(selector) && bracketCount > 0 {
				if selector[end] == '(' {
					bracketCount++
				} else if selector[end] == ')' {
					bracketCount--
				}
				end++
			}

			if bracketCount != 0 {
				return "", fmt.Errorf("unmatched brackets in count expression")
			}

			countXPath := selector[start+6 : end-1]
			count, err := e.evaluateCount(countXPath, element, doc)
			if err != nil {
				return "", err
			}

			selector = selector[:start] + fmt.Sprintf("%d", count) + selector[end:]
		}
		return selector, nil
	}

	if strings.HasPrefix(field.Name, "_id") || strings.HasPrefix(field.Name, "_time") {
		if field.Type == "nested" {
			// Nested ID handling remains unchanged
			nestedElement := element
			nestedItem := make(ExtractedItem)
			for _, nestedField := range field.Fields {
				nestedValue, err := e.extractField(nestedElement, nestedField, url, doc)
				if err != nil {
					continue
				}
				nestedItem[nestedField.Name] = nestedValue
			}
			if len(nestedItem) > 0 {
				return nestedItem, nil
			}
		}

		switch field.From {
		case FromURL:
			matches := regexp.MustCompile(field.Pattern).FindStringSubmatch(url)
			if len(matches) > 1 {
				return strings.Join(matches[1:], "/"), nil
			}
			return nil, fmt.Errorf("failed to extract from URL using pattern: %s", field.Pattern)
		case FromElement:
			if strings.Contains(field.Selector, "count(") {
				processedSelector, err := processCountExpression(field.Selector)
				if err != nil {
					return nil, err
				}
				field.Selector = processedSelector
			}

			el, _ := queryElement(field.Selector, element)
			if el == nil {
				return nil, fmt.Errorf("element not found for selector: %s", field.Selector)
			}
			text := htmlquery.InnerText(el)
			matches := regexp.MustCompile(field.Pattern).FindStringSubmatch(text)
			if len(matches) > 1 {
				return strings.Join(matches[1:], "/"), nil
			}
			return nil, fmt.Errorf("failed to extract from element using pattern: %s", field.Pattern)
		default:
			return nil, fmt.Errorf("unsupported from: %s", field.From)
		}
	}

	switch field.Type {
	case "text":
		if strings.Contains(field.Selector, "count(") {
			processedSelector, err := processCountExpression(field.Selector)
			if err != nil {
				return "", err
			}
			field.Selector = processedSelector
		}

		el, _ := queryElement(field.Selector, element)
		if el == nil {
			return "", fmt.Errorf("element not found for selector: %s", field.Selector)
		}

		text := htmlquery.InnerText(el)
		text = regexp.MustCompile(`[ \t]+`).ReplaceAllString(text, " ")
		lines := strings.Split(text, "\n")
		var nonEmptyLines []string
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				nonEmptyLines = append(nonEmptyLines, trimmed)
			}
		}
		return strings.Join(nonEmptyLines, "\n"), nil

	case "attribute":
		el, _ := queryElement(field.Selector, element)
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
		nestedElement, _ := queryElement(field.Selector, element)
		if nestedElement == nil {
			return nil, fmt.Errorf("nested element not found for selector: %s", field.Selector)
		}
		nestedItem := make(ExtractedItem)
		for _, nestedField := range field.Fields {
			nestedValue, err := e.extractField(nestedElement, nestedField, url, doc)
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
		elements, _ := queryElements(field.Selector, element)
		if len(elements) == 0 {
			return nil, fmt.Errorf("elements not found for selector: %s", field.Selector)
		}

		// Check for single text field case
		if len(field.Fields) == 1 && field.Fields[0].Type == "text" && field.Fields[0].Selector == "." {
			var items []string
			for _, el := range elements {
				value, err := e.extractField(el, field.Fields[0], url, doc)
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
				value, err := e.extractField(el, subField, url, doc)
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

func isValidXPath(xpath string) bool {
	bracketCount := 0
	for _, c := range xpath {
		if c == '(' {
			bracketCount++
		} else if c == ')' {
			bracketCount--
		}
		if bracketCount < 0 {
			return false
		}
	}
	return bracketCount == 0
}
