package languages

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

func init() {
	Register(&JSONParser{})
}

// JSONParser implements LanguageParser for JSON configuration files
type JSONParser struct{}

func (p *JSONParser) Name() string {
	return "json"
}

func (p *JSONParser) Extensions() []string {
	return []string{".json"}
}

func (p *JSONParser) CanParse(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".json")
}

func (p *JSONParser) IsTestFile(path string) bool {
	// JSON files are typically not test files, but check for test fixtures
	pathLower := strings.ToLower(path)
	return strings.Contains(pathLower, "/fixtures/") ||
		strings.Contains(pathLower, "/testdata/") ||
		strings.Contains(pathLower, "__mocks__") ||
		strings.HasSuffix(pathLower, ".test.json")
}

func (p *JSONParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
	var elements []CodeElement

	content, err := ReadFileContent(path)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(root, path)
	fileName := filepath.Base(path)

	// Try to parse JSON to extract structure info
	var jsonData interface{}
	jsonErr := json.Unmarshal([]byte(content), &jsonData)

	// Build content for embedding
	var contentBuilder strings.Builder
	contentBuilder.WriteString(fmt.Sprintf("JSON file: %s\n", fileName))

	// Detect file type based on filename
	jsonKind := p.detectJsonKind(fileName)
	contentBuilder.WriteString(fmt.Sprintf("Type: %s\n", jsonKind))

	// Extract structure info if valid JSON
	if jsonErr == nil {
		keys := p.extractTopLevelKeys(jsonData)
		if len(keys) > 0 {
			if len(keys) > 20 {
				keys = keys[:20]
				contentBuilder.WriteString(fmt.Sprintf("Top-level keys: %s (and more)\n", strings.Join(keys, ", ")))
			} else {
				contentBuilder.WriteString(fmt.Sprintf("Top-level keys: %s\n", strings.Join(keys, ", ")))
			}
		}
	} else {
		contentBuilder.WriteString("Warning: Invalid JSON\n")
	}

	// Include actual content (truncated)
	contentBuilder.WriteString("\n--- Content ---\n")
	contentBuilder.WriteString(TruncateContent(content, 4000))

	// Detect cross-cutting concerns
	concerns := DetectConcerns(relPath, content)
	tags := []string{"json", "config", jsonKind}
	tags = append(tags, concerns...)

	// Extract symbols (key-value pairs for config files)
	var symbols []Symbol
	if extractSymbols && jsonErr == nil {
		symbols = p.extractSymbols(jsonData, "")
	}

	element := CodeElement{
		Name:     fileName,
		Kind:     jsonKind,
		Path:     "/" + relPath,
		Content:  contentBuilder.String(),
		Package:  "config",
		FilePath: relPath,
		Tags:     tags,
		Concerns: concerns,
		Symbols:  symbols,
	}

	return []CodeElement{element}, nil
}

func (p *JSONParser) detectJsonKind(fileName string) string {
	fileNameLower := strings.ToLower(fileName)

	switch {
	case fileNameLower == "package.json":
		return "npm-package"
	case fileNameLower == "tsconfig.json" || strings.HasPrefix(fileNameLower, "tsconfig."):
		return "typescript-config"
	case fileNameLower == "composer.json":
		return "composer-package"
	case fileNameLower == "cargo.json":
		return "cargo-config"
	case strings.HasPrefix(fileNameLower, ".eslint"):
		return "eslint-config"
	case strings.HasPrefix(fileNameLower, ".prettier"):
		return "prettier-config"
	case fileNameLower == "manifest.json":
		return "manifest"
	case strings.Contains(fileNameLower, "config"):
		return "configuration"
	case strings.Contains(fileNameLower, "settings"):
		return "settings"
	default:
		return "json-data"
	}
}

func (p *JSONParser) extractTopLevelKeys(data interface{}) []string {
	var keys []string

	switch v := data.(type) {
	case map[string]interface{}:
		for k := range v {
			keys = append(keys, k)
		}
	case []interface{}:
		// For arrays, look at first element if it's an object
		if len(v) > 0 {
			if obj, ok := v[0].(map[string]interface{}); ok {
				for k := range obj {
					keys = append(keys, k)
				}
			}
		}
	}

	return keys
}

func (p *JSONParser) extractSymbols(data interface{}, prefix string) []Symbol {
	var symbols []Symbol

	switch v := data.(type) {
	case map[string]interface{}:
		for key, val := range v {
			fullKey := key
			if prefix != "" {
				fullKey = prefix + "." + key
			}

			// Extract string/number/bool values as symbols
			switch value := val.(type) {
			case string:
				if len(value) < 200 { // Only short values
					symbols = append(symbols, Symbol{
						Name:     fullKey,
						Type:     "config-value",
						Value:    value,
						Exported: true,
						Language: "json",
					})
				}
			case float64:
				symbols = append(symbols, Symbol{
					Name:     fullKey,
					Type:     "config-value",
					Value:    fmt.Sprintf("%v", value),
					Exported: true,
					Language: "json",
				})
			case bool:
				symbols = append(symbols, Symbol{
					Name:     fullKey,
					Type:     "config-value",
					Value:    fmt.Sprintf("%v", value),
					Exported: true,
					Language: "json",
				})
			case map[string]interface{}:
				// Recurse one level for nested configs (but not too deep)
				if prefix == "" {
					nested := p.extractSymbols(value, key)
					symbols = append(symbols, nested...)
				}
			}
		}
	}

	return symbols
}
