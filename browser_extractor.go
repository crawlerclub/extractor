package extractor

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

type BrowserExtractor struct {
	Config  ExtractorConfig
	Browser *rod.Browser
}

func NewBrowserExtractor(config ExtractorConfig) *BrowserExtractor {
	launcher := rod.New().ControlURL(launcher.New().Set("--no-sandbox").MustLaunch())
	browser := launcher.MustConnect()
	return &BrowserExtractor{Config: config, Browser: browser}
}

func (e *BrowserExtractor) Extract(url string) (*ExtractionResult, error) {
	result := &ExtractionResult{
		SchemaResults: make(map[string]SchemaResult),
		Errors:        make([]ExtractionError, 0),
	}

	page := e.Browser.MustPage(url)
	defer page.Close()

	page.MustWaitStable()

	result.FinalURL = page.MustInfo().URL

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

	return result, nil
}

func (e *BrowserExtractor) extractItemWithSchema(element *rod.Element, url string, schema Schema) (ExtractedItem, []ExtractionError) {
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

func (e *BrowserExtractor) extractField(element *rod.Element, field Field) (interface{}, error) {
	if strings.HasPrefix(field.Name, "_id") {
		// extract nested id
		if field.Type == "nested" {
			nestedElement, err := element.ElementX(".")
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
		}

		// if nested id not found, extract id from url or element
		var id string
		switch field.From {
		case FromURL:
			url := element.Page().MustInfo().URL
			matches := regexp.MustCompile(field.Pattern).FindStringSubmatch(url)
			if len(matches) > 1 {
				id = strings.Join(matches[1:], "/")
			} else {
				return nil, fmt.Errorf("failed to extract id from URL using pattern: %s", field.Pattern)
			}
		case FromElement:
			el, err := element.ElementX(field.Selector)
			if err != nil {
				return nil, fmt.Errorf("element not found for selector: %s", field.Selector)
			}
			text, err := el.Text()
			if err != nil {
				return nil, fmt.Errorf("failed to get text from element: %s", field.Selector)
			}
			matches := regexp.MustCompile(field.Pattern).FindStringSubmatch(text)
			if len(matches) > 1 {
				id = strings.Join(matches[1:], "/")
			} else {
				return nil, fmt.Errorf("failed to extract id from element using pattern: %s", field.Pattern)
			}
		default:
			return nil, fmt.Errorf("unsupported from: %s", field.From)
		}
		return id, nil
	}

	if strings.HasPrefix(field.Name, "_time") {
		// extract time from url or element
		var time string
		switch field.From {
		case FromURL:
			url := element.Page().MustInfo().URL
			matches := regexp.MustCompile(field.Pattern).FindStringSubmatch(url)
			if len(matches) > 1 {
				time = strings.Join(matches[1:], "/")
			} else {
				return nil, fmt.Errorf("failed to extract time from URL using pattern: %s", field.Pattern)
			}
		case FromElement:
			el, err := element.ElementX(field.Selector)
			if err != nil {
				return nil, fmt.Errorf("element not found for selector: %s", field.Selector)
			}
			text, err := el.Text()
			if err != nil {
				return nil, fmt.Errorf("failed to get text from element: %s", field.Selector)
			}
			matches := regexp.MustCompile(field.Pattern).FindStringSubmatch(text)
			if len(matches) > 1 {
				time = strings.Join(matches[1:], "/")
			} else {
				return nil, fmt.Errorf("failed to extract time from element using pattern: %s", field.Pattern)
			}
		default:
			return nil, fmt.Errorf("unsupported from: %s", field.From)
		}
		return time, nil
	}

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

		// 检查是否只有一个子字段
		if len(field.Fields) == 1 && field.Fields[0].Type == "text" && field.Fields[0].Selector == "." {
			// 简化输出为字符串数组
			var items []string
			for _, el := range elements {
				value, err := e.extractField(el, field.Fields[0])
				if err != nil {
					continue
				}
				if str, ok := value.(string); ok {
					items = append(items, str)
				}
			}
			return items, nil
		}

		// 原有的对象数组处理逻辑
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

func extractExternalID(item map[string]interface{}) (string, bool) {
	idItem, ok := item["_id"]
	if !ok {
		return "", false
	}

	switch idValue := idItem.(type) {
	case string:
		return idValue, true

	case ExtractedItem:
		type idField struct {
			FieldK string
			FieldV string
		}
		idFields := []idField{}
		for idk, idv := range idValue {
			if !strings.HasPrefix(idk, "_id") {
				continue
			}
			if fieldV, ok := idv.(string); ok {
				idFields = append(idFields, idField{
					FieldK: idk,
					FieldV: fieldV,
				})
			}
		}
		sort.Slice(idFields, func(i, j int) bool {
			return idFields[i].FieldK < idFields[j].FieldK
		})
		idParts := []string{}
		for _, idField := range idFields {
			idParts = append(idParts, idField.FieldV)
		}
		return strings.Join(idParts, "_"), true
	}

	return "", false
}

func extractExternalTime(item map[string]interface{}) (time.Time, bool) {
	timeItem, ok := item["_time"]
	if !ok {
		return time.Time{}, false
	}
	loc, err := time.LoadLocation("Asia/Hong_Kong")
	if err != nil {
		return time.Time{}, false
	}

	switch timeValue := timeItem.(type) {
	case string:
		if t, err := time.ParseInLocation("2006/01/02", timeValue, loc); err == nil {
			return t, true
		}
	case ExtractedItem:
		type timeField struct {
			FieldK string
			FieldV string
		}
		timeFields := []timeField{}
		for timek, timev := range timeValue {
			if !strings.HasPrefix(timek, "_time") {
				continue
			}
			if fieldV, ok := timev.(string); ok {
				timeFields = append(timeFields, timeField{
					FieldK: timek,
					FieldV: fieldV,
				})
			}
		}
		sort.Slice(timeFields, func(i, j int) bool {
			return timeFields[i].FieldK < timeFields[j].FieldK
		})
		timeParts := []string{}
		for _, timeField := range timeFields {
			timeParts = append(timeParts, timeField.FieldV)
		}
		if t, err := time.ParseInLocation("2006/01/02", strings.Join(timeParts, "/"), loc); err == nil {
			return t, true
		}
	}

	return time.Time{}, false
}
