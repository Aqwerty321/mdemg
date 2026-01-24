package symbols

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

func TestDebugAST(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.ts")

	content := `export class Foo {
	private static BAR = 42;
}
`
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	parser := sitter.NewParser()
	parser.SetLanguage(typescript.GetLanguage())

	tree, err := parser.ParseCtx(context.Background(), nil, []byte(content))
	if err != nil {
		t.Fatal(err)
	}

	// Print all node types
	var walk func(node *sitter.Node, depth int)
	walk = func(node *sitter.Node, depth int) {
		indent := ""
		for i := 0; i < depth; i++ {
			indent += "  "
		}
		nodeContent := node.Content([]byte(content))
		if len(nodeContent) > 50 {
			nodeContent = nodeContent[:50] + "..."
		}
		t.Logf("%s%s: %q", indent, node.Type(), nodeContent)
		for i := 0; i < int(node.ChildCount()); i++ {
			walk(node.Child(i), depth+1)
		}
	}
	walk(tree.RootNode(), 0)
}
