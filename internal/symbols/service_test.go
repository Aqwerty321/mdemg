package symbols

import (
	"context"
	"testing"
)

func TestServiceQuery(t *testing.T) {
	svc, err := NewService(DefaultParserConfig())
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	defer svc.Close()

	// Parse some test content
	content := []byte(`package test

const (
	DefaultTimeout = 5000
	MaxRetries = 3
	ConfigPath = "/etc/app/config.yaml"
)

func ProcessData(data string) bool {
	return len(data) > 0
}

func validateInput(input string) error {
	return nil
}
`)

	result, err := svc.parser.ParseContent(context.Background(), "test.go", LangGo, content)
	if err != nil {
		t.Fatalf("Failed to parse content: %v", err)
	}

	// Manually add to cache
	svc.cache.set("test.go", result)

	// Test Query with pattern
	resp, err := svc.Query(context.Background(), QueryRequest{
		Query: "Default",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if resp.TotalCount != 1 {
		t.Errorf("Expected 1 match for 'Default', got %d", resp.TotalCount)
	}
	if len(resp.Symbols) > 0 && resp.Symbols[0].Name != "DefaultTimeout" {
		t.Errorf("Expected DefaultTimeout, got %s", resp.Symbols[0].Name)
	}

	// Test Query with symbol type filter
	resp, err = svc.Query(context.Background(), QueryRequest{
		SymbolTypes: []SymbolType{SymbolTypeConst},
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if resp.TotalCount != 3 {
		t.Errorf("Expected 3 constants, got %d", resp.TotalCount)
	}

	// Test exported only filter
	resp, err = svc.Query(context.Background(), QueryRequest{
		SymbolTypes:  []SymbolType{SymbolTypeFunction},
		ExportedOnly: true,
		Limit:        10,
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	// Only ProcessData is exported (starts with uppercase)
	if resp.TotalCount != 1 {
		t.Errorf("Expected 1 exported function, got %d", resp.TotalCount)
	}
}

func TestServiceFindByName(t *testing.T) {
	svc, err := NewService(DefaultParserConfig())
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	defer svc.Close()

	content := []byte(`package test

const MaxRetries = 3
const maxRetries = 5  // private, different case
`)

	result, err := svc.parser.ParseContent(context.Background(), "test.go", LangGo, content)
	if err != nil {
		t.Fatalf("Failed to parse content: %v", err)
	}
	svc.cache.set("test.go", result)

	// Find exact name
	symbols, err := svc.FindByName(context.Background(), "MaxRetries")
	if err != nil {
		t.Fatalf("FindByName failed: %v", err)
	}

	if len(symbols) != 1 {
		t.Errorf("Expected 1 match for 'MaxRetries', got %d", len(symbols))
	}
}

func TestServiceFindConstants(t *testing.T) {
	svc, err := NewService(DefaultParserConfig())
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	defer svc.Close()

	content := []byte(`
export const API_URL = "https://api.example.com";
export const API_KEY = "secret123";
export const MAX_CONNECTIONS = 100;
export function apiCall() { return fetch(API_URL); }
`)

	result, err := svc.parser.ParseContent(context.Background(), "test.ts", LangTypeScript, content)
	if err != nil {
		t.Fatalf("Failed to parse content: %v", err)
	}
	svc.cache.set("test.ts", result)

	// Find constants starting with "API"
	symbols, err := svc.FindConstants(context.Background(), "API")
	if err != nil {
		t.Fatalf("FindConstants failed: %v", err)
	}

	if len(symbols) != 2 {
		t.Errorf("Expected 2 constants matching 'API', got %d", len(symbols))
	}
}

func TestServiceCacheStats(t *testing.T) {
	svc, err := NewService(DefaultParserConfig())
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	defer svc.Close()

	// Empty cache
	files, symbols := svc.CacheStats()
	if files != 0 || symbols != 0 {
		t.Errorf("Expected empty cache, got %d files, %d symbols", files, symbols)
	}

	// Add some content - use const block for reliable parsing
	content := []byte(`package test

const (
	Alpha = 1
	Beta = 2
)
`)
	result, _ := svc.parser.ParseContent(context.Background(), "test.go", LangGo, content)
	svc.cache.set("test.go", result)

	files, symbols = svc.CacheStats()
	if files != 1 {
		t.Errorf("Expected 1 file, got %d", files)
	}
	if symbols != 2 {
		t.Errorf("Expected 2 symbols, got %d", symbols)
	}

	// Clear cache
	svc.ClearCache()
	files, symbols = svc.CacheStats()
	if files != 0 || symbols != 0 {
		t.Errorf("Expected empty cache after clear, got %d files, %d symbols", files, symbols)
	}
}

func TestNoOpProvider(t *testing.T) {
	provider := &NoOpProvider{}

	// All methods should return empty results without error
	fs, err := provider.ParseFile(context.Background(), "test.go")
	if err != nil {
		t.Errorf("ParseFile should not error: %v", err)
	}
	if len(fs.Symbols) != 0 {
		t.Error("ParseFile should return empty symbols")
	}

	resp, err := provider.Query(context.Background(), QueryRequest{Query: "test"})
	if err != nil {
		t.Errorf("Query should not error: %v", err)
	}
	if resp.TotalCount != 0 {
		t.Error("Query should return zero count")
	}

	symbols, err := provider.FindByName(context.Background(), "test")
	if err != nil {
		t.Errorf("FindByName should not error: %v", err)
	}
	if len(symbols) != 0 {
		t.Error("FindByName should return empty slice")
	}
}

func TestToEvidence(t *testing.T) {
	sym := Symbol{
		Name:           "DEFAULT_TIMEOUT",
		Type:           SymbolTypeConst,
		Value:          "5000",
		Line:           10,
		Snippet:        "const DEFAULT_TIMEOUT = 5000",
		DocComment:     "Default timeout in ms",
		TypeAnnotation: "number",
	}

	ev := ToEvidence(sym)

	if ev.Name != sym.Name {
		t.Errorf("Name mismatch: %s != %s", ev.Name, sym.Name)
	}
	if ev.Type != string(sym.Type) {
		t.Errorf("Type mismatch: %s != %s", ev.Type, sym.Type)
	}
	if ev.Value != sym.Value {
		t.Errorf("Value mismatch: %s != %s", ev.Value, sym.Value)
	}
	if ev.Line != sym.Line {
		t.Errorf("Line mismatch: %d != %d", ev.Line, sym.Line)
	}
}

func TestToEvidenceList(t *testing.T) {
	symbols := []Symbol{
		{Name: "A", Type: SymbolTypeConst},
		{Name: "B", Type: SymbolTypeFunction},
	}

	evidence := ToEvidenceList(symbols)

	if len(evidence) != 2 {
		t.Fatalf("Expected 2 evidence items, got %d", len(evidence))
	}
	if evidence[0].Name != "A" {
		t.Errorf("Expected first item name 'A', got %s", evidence[0].Name)
	}
	if evidence[1].Name != "B" {
		t.Errorf("Expected second item name 'B', got %s", evidence[1].Name)
	}
}
