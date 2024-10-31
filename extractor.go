package extractor

import (
	"fmt"

	"github.com/go-rod/rod"
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

func NewExtractor(schema Schema) *Extractor {
	browser := rod.New().MustConnect()
	return &Extractor{Schema: schema, Browser: browser}
}

func (e *Extractor) Extract(url string) ([]ExtractedItem, error) {
	page := e.Browser.MustPage(url)
	defer page.Close()

	page.MustWaitStable()

	elements, err := page.ElementsX(e.Schema.BaseSelector)
	if err != nil {
		return nil, fmt.Errorf("failed to find elements with base selector: %v", err)
	}

	var items []ExtractedItem
	for _, element := range elements {
		item, err := e.extractItem(element)
		if err != nil {
			// Log the error and continue with the next item
			fmt.Printf("Error extracting item: %v\n", err)
			continue
		}
		items = append(items, item)
	}

	return items, nil
}

func (e *Extractor) extractItem(element *rod.Element) (ExtractedItem, error) {
	item := make(ExtractedItem)

	for _, field := range e.Schema.Fields {
		value, err := e.extractField(element, field)
		if err != nil {
			return nil, err
		}
		item[field.Name] = value
	}

	return item, nil
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
	default:
		return nil, fmt.Errorf("unsupported field type: %s", field.Type)
	}
}
