package extractor

const (
	FromURL     string = "url"
	FromElement string = "element"
)

type Extractor interface {
	Extract(url string) (*ExtractionResult, error)
	ExtractWithoutCache(url string) (*ExtractionResult, error)
}

func NewExtractor(config ExtractorConfig) Extractor {
	if config.Mode == "static" {
		return NewStaticExtractor(config)
	}
	return NewBrowserExtractor(config)
}

type ExtractorConfig struct {
	Name       string   `json:"name"`
	Pattern    string   `json:"pattern"`
	ExampleURL string   `json:"example_url"`
	Mode       string   `json:"mode"`
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
	From      string  `json:"from"`
	Selector  string  `json:"selector"`
	Pattern   string  `json:"pattern"`
	Type      string  `json:"type"`
	Attribute string  `json:"attribute,omitempty"`
	Fields    []Field `json:"fields,omitempty"`
}

type ExtractedItem map[string]interface{}

type ExtractionResult struct {
	SchemaResults map[string]SchemaResult
	Errors        []ExtractionError
	FinalURL      string
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
