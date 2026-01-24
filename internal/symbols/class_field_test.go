package symbols

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

func TestClassStaticFields(t *testing.T) {
	// Create test file with class static fields
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.ts")

	content := `export class AbstractStorageService {
	private static DEFAULT_FLUSH_INTERVAL = 60 * 1000;
	protected static BROWSER_DEFAULT = 5 * 1000;
	readonly instanceProp = 123;
}

export const TOP_LEVEL_CONST = 'hello';
`
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// First, verify tree-sitter finds the public_field_definition
	parser := sitter.NewParser()
	parser.SetLanguage(typescript.GetLanguage())
	tree, _ := parser.ParseCtx(context.Background(), nil, []byte(content))

	foundPublicField := false
	var walk func(node *sitter.Node)
	walk = func(node *sitter.Node) {
		if node.Type() == "public_field_definition" {
			foundPublicField = true
			t.Logf("Found public_field_definition: %s", node.Content([]byte(content)))
		}
		for i := 0; i < int(node.ChildCount()); i++ {
			walk(node.Child(i))
		}
	}
	walk(tree.RootNode())

	if !foundPublicField {
		t.Error("tree-sitter did not find public_field_definition")
	}

	cfg := ParserConfig{
		IncludeDocComments: true,
		EvaluateConstants:  true,
		IncludePrivate:     true, // Include non-exported symbols
	}
	svc, err := NewService(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	result, err := svc.ParseFile(context.Background(), testFile)
	if err != nil {
		t.Fatal(err)
	}

	// Check we got the expected symbols
	foundFlush := false
	foundBrowser := false
	foundTopLevel := false

	t.Logf("Total symbols found: %d", len(result.Symbols))
	for _, sym := range result.Symbols {
		t.Logf("Found: %s (type=%s, value=%s, line=%d)", sym.Name, sym.Type, sym.Value, sym.LineNumber)
		switch sym.Name {
		case "DEFAULT_FLUSH_INTERVAL":
			foundFlush = true
			if sym.Type != SymbolTypeConst {
				t.Errorf("DEFAULT_FLUSH_INTERVAL type = %s, want const", sym.Type)
			}
			if sym.Value != "60000" {
				t.Errorf("DEFAULT_FLUSH_INTERVAL value = %s, want 60000", sym.Value)
			}
		case "BROWSER_DEFAULT":
			foundBrowser = true
			if sym.Value != "5000" {
				t.Errorf("BROWSER_DEFAULT value = %s, want 5000", sym.Value)
			}
		case "TOP_LEVEL_CONST":
			foundTopLevel = true
		}
	}

	if !foundFlush {
		t.Error("Did not find DEFAULT_FLUSH_INTERVAL")
	}
	if !foundBrowser {
		t.Error("Did not find BROWSER_DEFAULT")
	}
	if !foundTopLevel {
		t.Error("Did not find TOP_LEVEL_CONST")
	}
}
