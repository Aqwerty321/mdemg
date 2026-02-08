package symbols

import (
	"context"
	"testing"
)

func TestQueryEngineCompilation(t *testing.T) {
	// Create parser to get languages map
	parser, err := NewParser(DefaultParserConfig())
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}
	defer parser.Close()

	// Query engine should have been initialized
	if parser.queryEngine == nil {
		t.Fatal("query engine was not initialized")
	}

	// Check that queries were loaded for each language
	expectedLanguages := []Language{
		LangGo, LangPython, LangTypeScript,
		LangRust, LangJava, LangC, LangCPP,
	}

	for _, lang := range expectedLanguages {
		queries, ok := parser.queryEngine.queries[lang]
		if !ok {
			t.Errorf("no queries loaded for language %s", lang)
			continue
		}
		if len(queries) == 0 {
			t.Errorf("zero queries loaded for language %s", lang)
		}
		t.Logf("Language %s: %d queries loaded", lang, len(queries))
	}
}

func TestGoImportExtraction(t *testing.T) {
	source := `package main

import (
	"fmt"
	"context"
	"github.com/example/pkg"
)

func main() {
	fmt.Println("hello")
}
`

	parser, err := NewParser(DefaultParserConfig())
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}
	defer parser.Close()

	result, err := parser.ParseContent(context.Background(), "test.go", LangGo, []byte(source))
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if len(result.Relationships) == 0 {
		t.Fatal("no relationships extracted")
	}

	// Check for import relationships
	importCount := 0
	for _, rel := range result.Relationships {
		if rel.Relation == RelImports {
			importCount++
			t.Logf("Import: %s -> %s (line %d)", rel.Source, rel.Target, rel.Line)
		}
	}

	if importCount < 3 {
		t.Errorf("expected at least 3 imports, got %d", importCount)
	}

	// Verify relationship structure
	for _, rel := range result.Relationships {
		if rel.Source == "" {
			t.Error("relationship has empty source")
		}
		if rel.Target == "" {
			t.Error("relationship has empty target")
		}
		if rel.Relation == "" {
			t.Error("relationship has empty relation")
		}
		if rel.Confidence == 0 {
			t.Error("relationship has zero confidence")
		}
		if rel.Tier == 0 {
			t.Error("relationship has zero tier")
		}
	}
}

func TestPythonImportExtraction(t *testing.T) {
	source := `import os
import sys
from typing import Optional, List

def main():
    print("hello")
`

	parser, err := NewParser(DefaultParserConfig())
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}
	defer parser.Close()

	result, err := parser.ParseContent(context.Background(), "test.py", LangPython, []byte(source))
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	importCount := 0
	for _, rel := range result.Relationships {
		if rel.Relation == RelImports {
			importCount++
			t.Logf("Import: %s -> %s", rel.Source, rel.Target)
		}
	}

	if importCount < 2 {
		t.Errorf("expected at least 2 imports, got %d", importCount)
	}
}

func TestTypeScriptImportExtraction(t *testing.T) {
	source := `import { Component } from 'react';
import * as fs from 'fs';
import utils from './utils';

function hello() {
    console.log("hello");
}
`

	parser, err := NewParser(DefaultParserConfig())
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}
	defer parser.Close()

	result, err := parser.ParseContent(context.Background(), "test.ts", LangTypeScript, []byte(source))
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	importCount := 0
	for _, rel := range result.Relationships {
		if rel.Relation == RelImports {
			importCount++
			// Verify quotes are stripped
			if len(rel.Target) > 0 && (rel.Target[0] == '"' || rel.Target[0] == '\'') {
				t.Errorf("import path still has quotes: %s", rel.Target)
			}
			t.Logf("Import: %s -> %s", rel.Source, rel.Target)
		}
	}

	if importCount < 3 {
		t.Errorf("expected at least 3 imports, got %d", importCount)
	}
}

func TestCallExtraction(t *testing.T) {
	source := `package main

import "fmt"

func helper() {
	fmt.Println("helper")
}

func main() {
	fmt.Println("hello")
	helper()
	fmt.Printf("world")
}
`

	parser, err := NewParser(DefaultParserConfig())
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}
	defer parser.Close()

	result, err := parser.ParseContent(context.Background(), "test.go", LangGo, []byte(source))
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	callCount := 0
	for _, rel := range result.Relationships {
		if rel.Relation == RelCalls {
			callCount++
			t.Logf("Call: %s (line %d)", rel.Target, rel.Line)
		}
	}

	if callCount == 0 {
		t.Error("expected at least some function calls")
	}

	// Verify call tier
	for _, rel := range result.Relationships {
		if rel.Relation == RelCalls && rel.Tier != 3 {
			t.Errorf("call relationship has wrong tier: %d (expected 3)", rel.Tier)
		}
	}
}

func TestCallExtractionCap(t *testing.T) {
	// Create a function with >50 calls to verify capping
	calls := ""
	for i := 0; i < 60; i++ {
		calls += "    fmt.Println(\"call\")\n"
	}

	source := `package main

import "fmt"

func main() {
` + calls + `}
`

	parser, err := NewParser(DefaultParserConfig())
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}
	defer parser.Close()

	result, err := parser.ParseContent(context.Background(), "test.go", LangGo, []byte(source))
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	callCount := 0
	for _, rel := range result.Relationships {
		if rel.Relation == RelCalls {
			callCount++
		}
	}

	// Should be capped at 50
	if callCount > 50 {
		t.Errorf("call count not capped: got %d, expected max 50", callCount)
	}

	t.Logf("Extracted %d calls (capped at 50)", callCount)
}

func TestInheritanceExtraction(t *testing.T) {
	source := `class Animal {
    name: string;
}

class Dog extends Animal {
    bark() {
        console.log("woof");
    }
}

interface Flyable {
    fly(): void;
}

class Bird implements Flyable {
    fly() {
        console.log("flying");
    }
}
`

	parser, err := NewParser(DefaultParserConfig())
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}
	defer parser.Close()

	result, err := parser.ParseContent(context.Background(), "test.ts", LangTypeScript, []byte(source))
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	extendsCount := 0
	implementsCount := 0
	for _, rel := range result.Relationships {
		switch rel.Relation {
		case RelExtends:
			extendsCount++
			t.Logf("Extends: %s -> %s", rel.Source, rel.Target)
		case RelImplements:
			implementsCount++
			t.Logf("Implements: %s -> %s", rel.Source, rel.Target)
		}
	}

	if extendsCount == 0 {
		t.Error("expected at least one extends relationship")
	}

	if implementsCount == 0 {
		t.Error("expected at least one implements relationship")
	}

	// Verify inheritance tier
	for _, rel := range result.Relationships {
		if (rel.Relation == RelExtends || rel.Relation == RelImplements) && rel.Tier != 2 {
			t.Errorf("inheritance relationship has wrong tier: %d (expected 2)", rel.Tier)
		}
	}
}

func TestPythonInheritanceExtraction(t *testing.T) {
	source := `class Animal:
    pass

class Dog(Animal):
    def bark(self):
        print("woof")
`

	parser, err := NewParser(DefaultParserConfig())
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}
	defer parser.Close()

	result, err := parser.ParseContent(context.Background(), "test.py", LangPython, []byte(source))
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	extendsCount := 0
	for _, rel := range result.Relationships {
		if rel.Relation == RelExtends {
			extendsCount++
			t.Logf("Extends: %s -> %s", rel.Source, rel.Target)
		}
	}

	if extendsCount == 0 {
		t.Error("expected at least one extends relationship")
	}
}
