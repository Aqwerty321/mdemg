package symbols

import (
	"context"
	"testing"
)

func TestParseTypeScriptConstants(t *testing.T) {
	parser, err := NewParser(DefaultParserConfig())
	if err != nil {
		t.Fatalf("Failed to create parser: %v", err)
	}
	defer parser.Close()

	content := []byte(`
// Default timeout in milliseconds
export const DEFAULT_TIMEOUT = 5000;

// Flush interval for storage
export const DEFAULT_FLUSH_INTERVAL = 60 * 1000;

const PRIVATE_CONST = 100;

export let mutableVar = "hello";
`)

	result, err := parser.ParseContent(context.Background(), "test.ts", LangTypeScript, content)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if len(result.Symbols) < 2 {
		t.Fatalf("Expected at least 2 exported symbols, got %d", len(result.Symbols))
	}

	// Check DEFAULT_TIMEOUT
	found := false
	for _, sym := range result.Symbols {
		if sym.Name == "DEFAULT_TIMEOUT" {
			found = true
			if sym.Type != SymbolTypeConst {
				t.Errorf("Expected type const, got %s", sym.Type)
			}
			if sym.Value != "5000" {
				t.Errorf("Expected value 5000, got %s", sym.Value)
			}
			if !sym.Exported {
				t.Error("Expected symbol to be exported")
			}
		}
	}
	if !found {
		t.Error("DEFAULT_TIMEOUT not found")
	}

	// Check DEFAULT_FLUSH_INTERVAL with expression evaluation
	for _, sym := range result.Symbols {
		if sym.Name == "DEFAULT_FLUSH_INTERVAL" {
			if sym.Value != "60000" {
				t.Errorf("Expected evaluated value 60000, got %s (raw: %s)", sym.Value, sym.RawValue)
			}
		}
	}

	// PRIVATE_CONST should not be in results (not exported)
	for _, sym := range result.Symbols {
		if sym.Name == "PRIVATE_CONST" {
			t.Error("PRIVATE_CONST should not be extracted (not exported)")
		}
	}
}

func TestParseTypeScriptFunctions(t *testing.T) {
	parser, err := NewParser(DefaultParserConfig())
	if err != nil {
		t.Fatalf("Failed to create parser: %v", err)
	}
	defer parser.Close()

	content := []byte(`
/**
 * Processes the input data
 * @param data The input data
 * @returns Processed result
 */
export function processData(data: string): boolean {
    return data.length > 0;
}

export async function fetchData(url: string): Promise<Response> {
    return fetch(url);
}

function privateHelper() {
    return true;
}
`)

	result, err := parser.ParseContent(context.Background(), "test.ts", LangTypeScript, content)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	// Check processData
	found := false
	for _, sym := range result.Symbols {
		if sym.Name == "processData" {
			found = true
			if sym.Type != SymbolTypeFunction {
				t.Errorf("Expected type function, got %s", sym.Type)
			}
			if !sym.Exported {
				t.Error("Expected function to be exported")
			}
		}
	}
	if !found {
		t.Error("processData not found")
	}

	// privateHelper should not be in results
	for _, sym := range result.Symbols {
		if sym.Name == "privateHelper" {
			t.Error("privateHelper should not be extracted (not exported)")
		}
	}
}

func TestParseTypeScriptClassAndInterface(t *testing.T) {
	parser, err := NewParser(DefaultParserConfig())
	if err != nil {
		t.Fatalf("Failed to create parser: %v", err)
	}
	defer parser.Close()

	content := []byte(`
export interface IStorage {
    get(key: string): string | null;
    set(key: string, value: string): void;
}

export class StorageService implements IStorage {
    private data: Map<string, string>;

    constructor() {
        this.data = new Map();
    }

    get(key: string): string | null {
        return this.data.get(key) ?? null;
    }

    set(key: string, value: string): void {
        this.data.set(key, value);
    }
}

export type StorageKey = string;

export enum StorageScope {
    APPLICATION = -1,
    PROFILE = 0,
    WORKSPACE = 1
}
`)

	result, err := parser.ParseContent(context.Background(), "test.ts", LangTypeScript, content)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	// Check for interface
	foundInterface := false
	foundClass := false
	foundType := false
	foundEnum := false

	for _, sym := range result.Symbols {
		switch sym.Name {
		case "IStorage":
			foundInterface = true
			if sym.Type != SymbolTypeInterface {
				t.Errorf("IStorage: expected interface, got %s", sym.Type)
			}
		case "StorageService":
			foundClass = true
			if sym.Type != SymbolTypeClass {
				t.Errorf("StorageService: expected class, got %s", sym.Type)
			}
		case "StorageKey":
			foundType = true
			if sym.Type != SymbolTypeType {
				t.Errorf("StorageKey: expected type, got %s", sym.Type)
			}
		case "StorageScope":
			foundEnum = true
			if sym.Type != SymbolTypeEnum {
				t.Errorf("StorageScope: expected enum, got %s", sym.Type)
			}
		}
	}

	if !foundInterface {
		t.Error("IStorage interface not found")
	}
	if !foundClass {
		t.Error("StorageService class not found")
	}
	if !foundType {
		t.Error("StorageKey type not found")
	}
	if !foundEnum {
		t.Error("StorageScope enum not found")
	}
}

func TestParseGoConstants(t *testing.T) {
	parser, err := NewParser(DefaultParserConfig())
	if err != nil {
		t.Fatalf("Failed to create parser: %v", err)
	}
	defer parser.Close()

	content := []byte(`package storage

// DefaultFlushInterval is the default flush interval in milliseconds.
const DefaultFlushInterval = 60 * 1000

const (
	// MaxRetries is the maximum number of retries.
	MaxRetries = 3
	// Timeout in seconds.
	Timeout = 30
)

const privateConst = 100

var GlobalCache = make(map[string]interface{})
`)

	result, err := parser.ParseContent(context.Background(), "test.go", LangGo, content)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	// Check DefaultFlushInterval
	found := false
	for _, sym := range result.Symbols {
		if sym.Name == "DefaultFlushInterval" {
			found = true
			if sym.Type != SymbolTypeConst {
				t.Errorf("Expected type const, got %s", sym.Type)
			}
			if sym.Value != "60000" {
				t.Errorf("Expected value 60000, got %s", sym.Value)
			}
			if !sym.Exported {
				t.Error("Expected symbol to be exported")
			}
		}
	}
	if !found {
		t.Error("DefaultFlushInterval not found")
	}

	// privateConst should not be in results (not exported - lowercase)
	for _, sym := range result.Symbols {
		if sym.Name == "privateConst" {
			t.Error("privateConst should not be extracted (not exported)")
		}
	}

	// Check GlobalCache var
	foundVar := false
	for _, sym := range result.Symbols {
		if sym.Name == "GlobalCache" {
			foundVar = true
			if sym.Type != SymbolTypeVar {
				t.Errorf("Expected type var, got %s", sym.Type)
			}
		}
	}
	if !foundVar {
		t.Error("GlobalCache var not found")
	}
}

func TestParseGoFunctionsAndTypes(t *testing.T) {
	parser, err := NewParser(DefaultParserConfig())
	if err != nil {
		t.Fatalf("Failed to create parser: %v", err)
	}
	defer parser.Close()

	content := []byte(`package storage

// Storage defines the storage interface.
type Storage interface {
	Get(key string) (string, error)
	Set(key string, value string) error
}

// StorageService implements Storage.
type StorageService struct {
	data map[string]string
}

// NewStorageService creates a new StorageService.
func NewStorageService() *StorageService {
	return &StorageService{
		data: make(map[string]string),
	}
}

// Get retrieves a value by key.
func (s *StorageService) Get(key string) (string, error) {
	val, ok := s.data[key]
	if !ok {
		return "", fmt.Errorf("key not found: %s", key)
	}
	return val, nil
}

func privateHelper() bool {
	return true
}
`)

	result, err := parser.ParseContent(context.Background(), "test.go", LangGo, content)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	foundInterface := false
	foundStruct := false
	foundFunction := false
	foundMethod := false

	for _, sym := range result.Symbols {
		switch sym.Name {
		case "Storage":
			foundInterface = true
			if sym.Type != SymbolTypeInterface {
				t.Errorf("Storage: expected interface, got %s", sym.Type)
			}
		case "StorageService":
			foundStruct = true
			if sym.Type != SymbolTypeStruct {
				t.Errorf("StorageService: expected struct, got %s", sym.Type)
			}
		case "NewStorageService":
			foundFunction = true
			if sym.Type != SymbolTypeFunction {
				t.Errorf("NewStorageService: expected function, got %s", sym.Type)
			}
		case "Get":
			foundMethod = true
			if sym.Type != SymbolTypeMethod {
				t.Errorf("Get: expected method, got %s", sym.Type)
			}
			if sym.Parent != "StorageService" {
				t.Errorf("Get: expected parent StorageService, got %s", sym.Parent)
			}
		case "privateHelper":
			t.Error("privateHelper should not be extracted (not exported)")
		}
	}

	if !foundInterface {
		t.Error("Storage interface not found")
	}
	if !foundStruct {
		t.Error("StorageService struct not found")
	}
	if !foundFunction {
		t.Error("NewStorageService function not found")
	}
	if !foundMethod {
		t.Error("Get method not found")
	}
}

func TestParsePythonConstants(t *testing.T) {
	parser, err := NewParser(DefaultParserConfig())
	if err != nil {
		t.Fatalf("Failed to create parser: %v", err)
	}
	defer parser.Close()

	content := []byte(`"""Storage module constants."""

# Default timeout in milliseconds
DEFAULT_TIMEOUT = 5000

# Maximum number of retries
MAX_RETRIES = 3

# Flush interval
FLUSH_INTERVAL = 60 * 1000

_private_var = "hidden"

regular_var = "not a constant"
`)

	result, err := parser.ParseContent(context.Background(), "test.py", LangPython, content)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	// Check DEFAULT_TIMEOUT
	found := false
	for _, sym := range result.Symbols {
		if sym.Name == "DEFAULT_TIMEOUT" {
			found = true
			if sym.Type != SymbolTypeConst {
				t.Errorf("Expected type const, got %s", sym.Type)
			}
			if sym.Value != "5000" {
				t.Errorf("Expected value 5000, got %s", sym.Value)
			}
			if !sym.Exported {
				t.Error("Expected symbol to be exported")
			}
		}
	}
	if !found {
		t.Error("DEFAULT_TIMEOUT not found")
	}

	// Check FLUSH_INTERVAL evaluation
	for _, sym := range result.Symbols {
		if sym.Name == "FLUSH_INTERVAL" {
			if sym.Value != "60000" {
				t.Errorf("Expected evaluated value 60000, got %s", sym.Value)
			}
		}
	}

	// _private_var should not be in results
	for _, sym := range result.Symbols {
		if sym.Name == "_private_var" {
			t.Error("_private_var should not be extracted (starts with _)")
		}
	}
}

func TestParsePythonFunctionsAndClasses(t *testing.T) {
	parser, err := NewParser(DefaultParserConfig())
	if err != nil {
		t.Fatalf("Failed to create parser: %v", err)
	}
	defer parser.Close()

	content := []byte(`"""Storage module."""

from typing import Optional


class StorageService:
    """A service for storing key-value data."""

    def __init__(self) -> None:
        self.data = {}

    def get(self, key: str) -> Optional[str]:
        """Retrieve a value by key."""
        return self.data.get(key)

    def set(self, key: str, value: str) -> None:
        """Store a value."""
        self.data[key] = value


def create_storage() -> StorageService:
    """Create a new storage service."""
    return StorageService()


def _private_helper() -> bool:
    return True
`)

	result, err := parser.ParseContent(context.Background(), "test.py", LangPython, content)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	foundClass := false
	foundFunction := false

	for _, sym := range result.Symbols {
		switch sym.Name {
		case "StorageService":
			foundClass = true
			if sym.Type != SymbolTypeClass {
				t.Errorf("StorageService: expected class, got %s", sym.Type)
			}
			if sym.DocComment == "" {
				t.Log("Warning: no docstring extracted for StorageService")
			}
		case "create_storage":
			foundFunction = true
			if sym.Type != SymbolTypeFunction {
				t.Errorf("create_storage: expected function, got %s", sym.Type)
			}
		case "_private_helper":
			t.Error("_private_helper should not be extracted (starts with _)")
		}
	}

	if !foundClass {
		t.Error("StorageService class not found")
	}
	if !foundFunction {
		t.Error("create_storage function not found")
	}
}

func TestLanguageFromExtension(t *testing.T) {
	tests := []struct {
		ext      string
		expected Language
	}{
		{".ts", LangTypeScript},
		{".tsx", LangTypeScript},
		{".js", LangJavaScript},
		{".jsx", LangJavaScript},
		{".go", LangGo},
		{".py", LangPython},
		{".pyi", LangPython},
		{".rs", LangRust},
		{".txt", LangUnknown},
		{".md", LangUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			got := LanguageFromExtension(tt.ext)
			if got != tt.expected {
				t.Errorf("LanguageFromExtension(%s) = %s, want %s", tt.ext, got, tt.expected)
			}
		})
	}
}

func TestConfigIncludePrivate(t *testing.T) {
	config := DefaultParserConfig()
	config.IncludePrivate = true

	parser, err := NewParser(config)
	if err != nil {
		t.Fatalf("Failed to create parser: %v", err)
	}
	defer parser.Close()

	content := []byte(`
const privateConst = 100;
export const publicConst = 200;
`)

	result, err := parser.ParseContent(context.Background(), "test.ts", LangTypeScript, content)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	foundPrivate := false
	foundPublic := false

	for _, sym := range result.Symbols {
		if sym.Name == "privateConst" {
			foundPrivate = true
		}
		if sym.Name == "publicConst" {
			foundPublic = true
		}
	}

	if !foundPrivate {
		t.Error("With IncludePrivate=true, privateConst should be extracted")
	}
	if !foundPublic {
		t.Error("publicConst should be extracted")
	}
}

func TestConfigMaxSymbolsPerFile(t *testing.T) {
	config := DefaultParserConfig()
	config.MaxSymbolsPerFile = 2

	parser, err := NewParser(config)
	if err != nil {
		t.Fatalf("Failed to create parser: %v", err)
	}
	defer parser.Close()

	// Use Go const block for reliable parsing
	content := []byte(`package test

const (
	Alpha = 1
	Beta = 2
	Gamma = 3
	Delta = 4
)
`)

	result, err := parser.ParseContent(context.Background(), "test.go", LangGo, content)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	t.Logf("Extracted %d symbols, ParseErrors: %v", len(result.Symbols), result.ParseErrors)
	for _, sym := range result.Symbols {
		t.Logf("  - %s (exported: %v)", sym.Name, sym.Exported)
	}

	if len(result.Symbols) > 2 {
		t.Errorf("Expected max 2 symbols, got %d", len(result.Symbols))
	}

	// With 4 exported Go consts and max=2, we should get a truncation warning
	if len(result.Symbols) == 2 && len(result.ParseErrors) == 0 {
		t.Error("Expected truncation warning in ParseErrors when symbols were truncated")
	}
}
