package extractor

import (
	"fmt"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

type Schema struct {
	Name         string  `json:"name"`
	BaseSelector string  `json:"baseSelector"`
	Fields       []Field `json:"fields"`
}

type Field struct {
	Name      string  `json:"name"`
	Selector  string  `json:"selector"`
	Type      string  `json:"type"`
	Attribute string  `json:"attribute,omitempty"`
	Fields    []Field `json:"fields,omitempty"`
}

type Extractor struct {
	Schema  Schema
	Browser *rod.Browser
}

type ExtractedItem map[string]interface{}

type ExtractionResult struct {
	Items  []ExtractedItem
	Errors []ExtractionError
}

type ExtractionError struct {
	Field   string
	Message string
	URL     string
}

func NewExtractor(schema Schema) *Extractor {
	launcher := rod.New().ControlURL(launcher.New().Set("--no-sandbox").MustLaunch())
	browser := launcher.MustConnect()
	return &Extractor{Schema: schema, Browser: browser}
}

func (e *Extractor) Extract(url string) (*ExtractionResult, error) {
	result := &ExtractionResult{
		Items:  make([]ExtractedItem, 0),
		Errors: make([]ExtractionError, 0),
	}

	page := e.Browser.MustPage(url)
	defer page.Close()

	page.MustWaitStable()

	elements, err := page.ElementsX(e.Schema.BaseSelector)
	if err != nil {
		return nil, fmt.Errorf("failed to find elements with base selector: %v", err)
	}

	for _, element := range elements {
		item, errs := e.extractItem(element, url)
		if len(errs) > 0 {
			result.Errors = append(result.Errors, errs...)
		}
		if item != nil {
			result.Items = append(result.Items, item)
		}
	}

	return result, nil
}

func (e *Extractor) extractItem(element *rod.Element, url string) (ExtractedItem, []ExtractionError) {
	item := make(ExtractedItem)
	var errors []ExtractionError

	for _, field := range e.Schema.Fields {
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
				return nil, err
			}
			nestedItem[nestedField.Name] = nestedValue
		}
		return nestedItem, nil
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
