package extractor

import (
	"fmt"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

type ExtractorConfig struct {
	Name       string   `json:"name"`
	Pattern    string   `json:"pattern"`
	ExampleURL string   `json:"example_url"`
	Schemas    []Schema `json:"schemas"`
}

type Schema struct {
	Name       string  `json:"name"`
	EntityType string  `json:"entity_type"`
	Selector   string  `json:"selector"`
	Type       string  `json:"type"`
	Fields     []Field `json:"fields,omitempty"`
}

type Field struct {
	Name      string  `json:"name"`
	Selector  string  `json:"selector"`
	Type      string  `json:"type"`
	Attribute string  `json:"attribute,omitempty"`
	Fields    []Field `json:"fields,omitempty"`
}

type Extractor struct {
	Config  ExtractorConfig
	Browser *rod.Browser
}

type ExtractedItem map[string]interface{}

type ExtractionResult struct {
	SchemaResults map[string]SchemaResult
	Errors        []ExtractionError
}

type SchemaResult struct {
	Schema SchemaInfo
	Items  []ExtractedItem
}

type SchemaInfo struct {
	Name       string `json:"name"`
	EntityType string `json:"entity_type"`
}

type ExtractionError struct {
	Field   string
	Message string
	URL     string
}

func NewExtractor(config ExtractorConfig) *Extractor {
	launcher := rod.New().ControlURL(launcher.New().Set("--no-sandbox").MustLaunch())
	browser := launcher.MustConnect()
	return &Extractor{Config: config, Browser: browser}
}

func (e *Extractor) Extract(url string) (*ExtractionResult, error) {
	result := &ExtractionResult{
		SchemaResults: make(map[string]SchemaResult),
		Errors:        make([]ExtractionError, 0),
	}

	page := e.Browser.MustPage(url)
	defer page.Close()

	page.MustWaitStable()

	// Extract items for each schema
	for _, schema := range e.Config.Schemas {
		schemaResult := SchemaResult{
			Schema: SchemaInfo{
				Name:       schema.Name,
				EntityType: schema.EntityType,
			},
			Items: make([]ExtractedItem, 0),
		}

		elements, err := page.ElementsX(schema.Selector)
		if err != nil {
			result.Errors = append(result.Errors, ExtractionError{
				Field:   schema.Name,
				Message: fmt.Sprintf("failed to find elements with selector: %s", schema.Selector),
				URL:     url,
			})
			continue
		}

		for _, element := range elements {
			item, errs := e.extractItemWithSchema(element, url, schema)
			if len(errs) > 0 {
				result.Errors = append(result.Errors, errs...)
			}
			if item != nil {
				schemaResult.Items = append(schemaResult.Items, item)
			}
		}

		result.SchemaResults[schema.Name] = schemaResult
	}

	return result, nil
}

func (e *Extractor) extractItemWithSchema(element *rod.Element, url string, schema Schema) (ExtractedItem, []ExtractionError) {
	item := make(ExtractedItem)
	var errors []ExtractionError

	for _, field := range schema.Fields {
		value, err := e.extractField(element, field)
		if err != nil {
			errors = append(errors, ExtractionError{
				Field:   field.Name,
				Message: err.Error(),
				URL:     url,
			})
			continue
		}
		item[field.Name] = value
	}

	return item, errors
}

func (e *Extractor) extractField(element *rod.Element, field Field) (interface{}, error) {
	switch field.Type {
	case "text":
		el, err := element.ElementX(field.Selector)
		if err != nil {
			return "", fmt.Errorf("element not found for selector: %s", field.Selector)
		}
		return el.Text()
	case "attribute":
		el, err := element.ElementX(field.Selector)
		if err != nil {
			return "", fmt.Errorf("element not found for selector: %s", field.Selector)
		}
		return el.Attribute(field.Attribute)
	case "nested":
		nestedElement, err := element.ElementX(field.Selector)
		if err != nil {
			return nil, fmt.Errorf("nested element not found for selector: %s", field.Selector)
		}
		nestedItem := make(ExtractedItem)
		for _, nestedField := range field.Fields {
			nestedValue, err := e.extractField(nestedElement, nestedField)
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
		elements, err := element.ElementsX(field.Selector)
		if err != nil {
			return nil, fmt.Errorf("elements not found for selector: %s", field.Selector)
		}

		var items []map[string]interface{}
		for _, el := range elements {
			item := make(map[string]interface{})
			for _, subField := range field.Fields {
				value, err := e.extractField(el, subField)
				if err != nil {
					continue
				}
				item[subField.Name] = value
			}
			items = append(items, item)
		}
		return items, nil
	default:
		return nil, fmt.Errorf("unsupported field type: %s", field.Type)
	}
}
