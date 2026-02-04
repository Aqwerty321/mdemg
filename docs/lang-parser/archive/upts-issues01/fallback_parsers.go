//go:build ignore

// fallback_parsers.go
// Fallback to config parsers when tree-sitter grammar is unavailable
//
// Add this file to: cmd/extract-symbols/fallback_parsers.go
// Then modify main.go to call TryFallbackParser when tree-sitter fails

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Symbol matches the output format expected by UPTS
type Symbol struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Line       int    `json:"line"`
	Exported   bool   `json:"exported"`
	Parent     string `json:"parent,omitempty"`
	Signature  string `json:"signature,omitempty"`
	Value      string `json:"value,omitempty"`
	DocComment string `json:"doc_comment,omitempty"`
}

// SymbolOutput is the JSON structure for parser output
type SymbolOutput struct {
	Symbols []Symbol `json:"symbols"`
}

// TryFallbackParser attempts to parse using config parsers
// Returns (symbols, handled, error)
func TryFallbackParser(filePath string) ([]Symbol, bool, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	base := strings.ToLower(filepath.Base(filePath))

	// Route to appropriate parser
	switch {
	case ext == ".yaml" || ext == ".yml":
		return parseYAML(filePath)
	case ext == ".toml":
		return parseTOML(filePath)
	case ext == ".json" || ext == ".jsonc":
		return parseJSON(filePath)
	case ext == ".env" || ext == ".ini" || ext == ".cfg" || ext == ".properties":
		return parseINI(filePath)
	case ext == ".sh" || ext == ".bash" || ext == ".zsh":
		return parseShell(filePath)
	case ext == ".sql":
		return parseSQL(filePath)
	case ext == ".cypher" || ext == ".cql":
		return parseCypher(filePath)
	case strings.HasPrefix(base, "dockerfile") || ext == ".dockerfile":
		return parseDockerfile(filePath)
	default:
		return nil, false, nil // Not handled
	}
}

// ============================================================
// YAML Parser
// ============================================================

func parseYAML(filePath string) ([]Symbol, bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, true, err
	}

	var symbols []Symbol
	lines := strings.Split(string(content), "\n")

	// Track current section context
	var currentSection string
	sectionIndent := -1

	keyPattern := regexp.MustCompile(`^(\s*)([a-zA-Z_][a-zA-Z0-9_.-]*):\s*(.*)$`)
	anchorPattern := regexp.MustCompile(`&(\w+)`)

	for i, line := range lines {
		lineNum := i + 1

		// Skip comments and empty lines
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		match := keyPattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}

		indent := len(match[1])
		key := match[2]
		value := strings.TrimSpace(match[3])

		// Detect section vs value
		isSection := value == "" || strings.HasPrefix(value, "&") || strings.HasPrefix(value, "|") || strings.HasPrefix(value, ">")

		// Update section context
		if indent <= sectionIndent || sectionIndent == -1 {
			if isSection {
				currentSection = key
				sectionIndent = indent
			} else {
				currentSection = ""
				sectionIndent = -1
			}
		}

		// Check for anchor
		docComment := ""
		if anchorMatch := anchorPattern.FindStringSubmatch(value); anchorMatch != nil {
			docComment = "anchor: " + anchorMatch[1]
		}

		sym := Symbol{
			Name:     key,
			Line:     lineNum,
			Exported: true,
		}

		if isSection {
			sym.Type = "section"
			if currentSection != "" && key != currentSection {
				sym.Name = currentSection + "." + key
				sym.Parent = currentSection
			}
		} else {
			sym.Type = "constant"
			sym.Value = cleanYAMLValue(value)
			if currentSection != "" {
				sym.Name = currentSection + "." + key
				sym.Parent = currentSection
			}
		}

		if docComment != "" {
			sym.DocComment = docComment
		}

		symbols = append(symbols, sym)
	}

	return symbols, true, nil
}

func cleanYAMLValue(value string) string {
	// Remove quotes
	if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
		(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
		return value[1 : len(value)-1]
	}
	// Remove inline comments
	if idx := strings.Index(value, " #"); idx > 0 {
		return strings.TrimSpace(value[:idx])
	}
	return value
}

// ============================================================
// TOML Parser
// ============================================================

func parseTOML(filePath string) ([]Symbol, bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, true, err
	}

	var symbols []Symbol
	lines := strings.Split(string(content), "\n")

	var currentSection string
	tablePattern := regexp.MustCompile(`^\[([^\]]+)\]$`)
	arrayTablePattern := regexp.MustCompile(`^\[\[([^\]]+)\]\]$`)
	keyValuePattern := regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_.-]*)\s*=\s*(.+)$`)

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		// Skip comments and empty lines
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Array of tables [[name]]
		if match := arrayTablePattern.FindStringSubmatch(trimmed); match != nil {
			currentSection = match[1]
			symbols = append(symbols, Symbol{
				Name:       currentSection,
				Type:       "section",
				Line:       lineNum,
				Exported:   true,
				DocComment: "array of tables",
			})
			continue
		}

		// Table [name]
		if match := tablePattern.FindStringSubmatch(trimmed); match != nil {
			currentSection = match[1]
			symbols = append(symbols, Symbol{
				Name:     currentSection,
				Type:     "section",
				Line:     lineNum,
				Exported: true,
			})
			continue
		}

		// Key = value
		if match := keyValuePattern.FindStringSubmatch(trimmed); match != nil {
			key := match[1]
			value := cleanTOMLValue(match[2])

			sym := Symbol{
				Name:     key,
				Type:     "constant",
				Line:     lineNum,
				Value:    value,
				Exported: true,
			}

			if currentSection != "" {
				sym.Name = currentSection + "." + key
				sym.Parent = currentSection
			}

			symbols = append(symbols, sym)
		}
	}

	return symbols, true, nil
}

func cleanTOMLValue(value string) string {
	value = strings.TrimSpace(value)
	// Remove quotes
	if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
		(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
		return value[1 : len(value)-1]
	}
	return value
}

// ============================================================
// JSON Parser
// ============================================================

func parseJSON(filePath string) ([]Symbol, bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, true, err
	}

	// Strip JSONC comments
	jsonContent := stripJSONComments(string(content))

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(jsonContent), &data); err != nil {
		return nil, true, fmt.Errorf("JSON parse error: %w", err)
	}

	var symbols []Symbol
	lines := strings.Split(string(content), "\n")

	// Walk JSON structure
	walkJSON(data, "", &symbols, lines)

	return symbols, true, nil
}

func walkJSON(data map[string]interface{}, prefix string, symbols *[]Symbol, lines []string) {
	for key, value := range data {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}

		lineNum := findJSONKeyLine(key, lines)

		switch v := value.(type) {
		case map[string]interface{}:
			*symbols = append(*symbols, Symbol{
				Name:     fullKey,
				Type:     "section",
				Line:     lineNum,
				Parent:   prefix,
				Exported: true,
			})
			walkJSON(v, fullKey, symbols, lines)
		case []interface{}:
			*symbols = append(*symbols, Symbol{
				Name:     fullKey,
				Type:     "constant",
				Line:     lineNum,
				Parent:   prefix,
				Exported: true,
				Value:    fmt.Sprintf("[%d items]", len(v)),
			})
		default:
			*symbols = append(*symbols, Symbol{
				Name:     fullKey,
				Type:     "constant",
				Line:     lineNum,
				Parent:   prefix,
				Exported: true,
				Value:    fmt.Sprintf("%v", v),
			})
		}
	}
}

func findJSONKeyLine(key string, lines []string) int {
	pattern := regexp.MustCompile(fmt.Sprintf(`"(%s)"\s*:`, regexp.QuoteMeta(key)))
	for i, line := range lines {
		if pattern.MatchString(line) {
			return i + 1
		}
	}
	return 1
}

func stripJSONComments(content string) string {
	// Remove // comments
	singleLineComment := regexp.MustCompile(`//.*$`)
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = singleLineComment.ReplaceAllString(line, "")
	}
	return strings.Join(lines, "\n")
}

// ============================================================
// INI/dotenv Parser
// ============================================================

func parseINI(filePath string) ([]Symbol, bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, true, err
	}

	var symbols []Symbol
	lines := strings.Split(string(content), "\n")

	var currentSection string
	sectionPattern := regexp.MustCompile(`^\[([^\]]+)\]$`)
	keyValuePattern := regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_.-]*)\s*=\s*(.*)$`)

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		// Skip comments and empty lines
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}

		// Section [name]
		if match := sectionPattern.FindStringSubmatch(trimmed); match != nil {
			currentSection = match[1]
			symbols = append(symbols, Symbol{
				Name:     currentSection,
				Type:     "section",
				Line:     lineNum,
				Exported: true,
			})
			continue
		}

		// Key = value
		if match := keyValuePattern.FindStringSubmatch(trimmed); match != nil {
			key := match[1]
			value := cleanINIValue(match[2])

			// Determine if exported (uppercase = env var style)
			exported := key == strings.ToUpper(key)

			sym := Symbol{
				Name:     key,
				Type:     "constant",
				Line:     lineNum,
				Value:    value,
				Exported: exported,
			}

			if currentSection != "" {
				sym.Name = currentSection + "." + key
				sym.Parent = currentSection
			}

			symbols = append(symbols, sym)
		}
	}

	return symbols, true, nil
}

func cleanINIValue(value string) string {
	value = strings.TrimSpace(value)
	// Remove quotes
	if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
		(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
		return value[1 : len(value)-1]
	}
	return value
}

// ============================================================
// Shell Parser
// ============================================================

func parseShell(filePath string) ([]Symbol, bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, true, err
	}

	var symbols []Symbol
	lines := strings.Split(string(content), "\n")

	exportPattern := regexp.MustCompile(`^export\s+([A-Za-z_][A-Za-z0-9_]*)=(.*)$`)
	readonlyPattern := regexp.MustCompile(`^readonly\s+([A-Za-z_][A-Za-z0-9_]*)=(.*)$`)
	funcPattern := regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_]*)\s*\(\)\s*\{?`)
	funcPattern2 := regexp.MustCompile(`^function\s+([a-zA-Z_][a-zA-Z0-9_]*)`)

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		// Skip comments and empty lines
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// export VAR=value
		if match := exportPattern.FindStringSubmatch(trimmed); match != nil {
			symbols = append(symbols, Symbol{
				Name:     match[1],
				Type:     "constant",
				Line:     lineNum,
				Value:    cleanShellValue(match[2]),
				Exported: true,
			})
			continue
		}

		// readonly VAR=value
		if match := readonlyPattern.FindStringSubmatch(trimmed); match != nil {
			symbols = append(symbols, Symbol{
				Name:     match[1],
				Type:     "constant",
				Line:     lineNum,
				Value:    cleanShellValue(match[2]),
				Exported: false,
			})
			continue
		}

		// function name() or name()
		if match := funcPattern.FindStringSubmatch(trimmed); match != nil {
			symbols = append(symbols, Symbol{
				Name:     match[1],
				Type:     "function",
				Line:     lineNum,
				Exported: true,
			})
			continue
		}

		if match := funcPattern2.FindStringSubmatch(trimmed); match != nil {
			symbols = append(symbols, Symbol{
				Name:     match[1],
				Type:     "function",
				Line:     lineNum,
				Exported: true,
			})
			continue
		}
	}

	return symbols, true, nil
}

func cleanShellValue(value string) string {
	value = strings.TrimSpace(value)
	// Remove quotes
	if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
		(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
		return value[1 : len(value)-1]
	}
	return value
}

// ============================================================
// Dockerfile Parser
// ============================================================

func parseDockerfile(filePath string) ([]Symbol, bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, true, err
	}

	var symbols []Symbol
	lines := strings.Split(string(content), "\n")

	argPattern := regexp.MustCompile(`^ARG\s+([A-Za-z_][A-Za-z0-9_]*)(?:=(.*))?$`)
	envPattern := regexp.MustCompile(`^ENV\s+([A-Za-z_][A-Za-z0-9_]*)(?:=(.*))?$`)
	fromPattern := regexp.MustCompile(`^FROM\s+(\S+)(?:\s+AS\s+(\S+))?`)
	exposePattern := regexp.MustCompile(`^EXPOSE\s+(\d+)`)
	entrypointPattern := regexp.MustCompile(`^ENTRYPOINT\s+(.+)$`)
	cmdPattern := regexp.MustCompile(`^CMD\s+(.+)$`)

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		// Skip comments and empty lines
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// ARG name=value
		if match := argPattern.FindStringSubmatch(trimmed); match != nil {
			symbols = append(symbols, Symbol{
				Name:     match[1],
				Type:     "constant",
				Line:     lineNum,
				Value:    match[2],
				Exported: true,
			})
			continue
		}

		// ENV name=value
		if match := envPattern.FindStringSubmatch(trimmed); match != nil {
			symbols = append(symbols, Symbol{
				Name:     match[1],
				Type:     "constant",
				Line:     lineNum,
				Value:    match[2],
				Exported: true,
			})
			continue
		}

		// FROM image AS stage
		if match := fromPattern.FindStringSubmatch(trimmed); match != nil {
			stageName := match[2]
			if stageName == "" {
				stageName = "base"
			}
			symbols = append(symbols, Symbol{
				Name:       stageName,
				Type:       "section",
				Line:       lineNum,
				Exported:   true,
				DocComment: "FROM " + match[1],
			})
			continue
		}

		// EXPOSE port
		if match := exposePattern.FindStringSubmatch(trimmed); match != nil {
			symbols = append(symbols, Symbol{
				Name:     "EXPOSE:" + match[1],
				Type:     "constant",
				Line:     lineNum,
				Value:    match[1],
				Exported: true,
			})
			continue
		}

		// ENTRYPOINT
		if match := entrypointPattern.FindStringSubmatch(trimmed); match != nil {
			symbols = append(symbols, Symbol{
				Name:      "ENTRYPOINT",
				Type:      "function",
				Line:      lineNum,
				Signature: cleanDockerCommand(match[1]),
				Exported:  true,
			})
			continue
		}

		// CMD
		if match := cmdPattern.FindStringSubmatch(trimmed); match != nil {
			symbols = append(symbols, Symbol{
				Name:      "CMD",
				Type:      "function",
				Line:      lineNum,
				Signature: cleanDockerCommand(match[1]),
				Exported:  true,
			})
			continue
		}
	}

	return symbols, true, nil
}

func cleanDockerCommand(cmd string) string {
	// Parse JSON array format ["cmd", "arg1", "arg2"]
	if strings.HasPrefix(cmd, "[") {
		var parts []string
		if err := json.Unmarshal([]byte(cmd), &parts); err == nil {
			return strings.Join(parts, " ")
		}
	}
	return cmd
}

// ============================================================
// SQL Parser
// ============================================================

func parseSQL(filePath string) ([]Symbol, bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, true, err
	}

	var symbols []Symbol
	lines := strings.Split(string(content), "\n")
	contentLower := strings.ToLower(string(content))

	// Patterns (case-insensitive)
	tablePattern := regexp.MustCompile(`(?i)CREATE\s+TABLE\s+(\w+)`)
	indexPattern := regexp.MustCompile(`(?i)CREATE\s+INDEX\s+(\w+)\s+.*ON\s+(\w+)`)
	viewPattern := regexp.MustCompile(`(?i)CREATE\s+(?:OR\s+REPLACE\s+)?VIEW\s+(\w+)`)
	funcPattern := regexp.MustCompile(`(?i)CREATE\s+(?:OR\s+REPLACE\s+)?FUNCTION\s+(\w+)`)
	triggerPattern := regexp.MustCompile(`(?i)CREATE\s+TRIGGER\s+(\w+).*ON\s+(\w+)`)
	typePattern := regexp.MustCompile(`(?i)CREATE\s+TYPE\s+(\w+)\s+AS\s+ENUM`)
	seqPattern := regexp.MustCompile(`(?i)CREATE\s+SEQUENCE\s+(\w+)`)

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)
		trimmedLower := strings.ToLower(trimmed)

		// Skip comments
		if strings.HasPrefix(trimmed, "--") {
			continue
		}

		// CREATE TABLE
		if match := tablePattern.FindStringSubmatch(trimmed); match != nil {
			symbols = append(symbols, Symbol{
				Name:     match[1],
				Type:     "table",
				Line:     lineNum,
				Exported: true,
			})
			continue
		}

		// CREATE INDEX
		if match := indexPattern.FindStringSubmatch(trimmed); match != nil {
			symbols = append(symbols, Symbol{
				Name:     match[1],
				Type:     "index",
				Line:     lineNum,
				Parent:   match[2],
				Exported: true,
			})
			continue
		}

		// CREATE VIEW
		if match := viewPattern.FindStringSubmatch(trimmed); match != nil {
			symbols = append(symbols, Symbol{
				Name:     match[1],
				Type:     "view",
				Line:     lineNum,
				Exported: true,
			})
			continue
		}

		// CREATE FUNCTION
		if match := funcPattern.FindStringSubmatch(trimmed); match != nil {
			symbols = append(symbols, Symbol{
				Name:     match[1],
				Type:     "function",
				Line:     lineNum,
				Exported: true,
			})
			continue
		}

		// CREATE TRIGGER
		if match := triggerPattern.FindStringSubmatch(trimmed); match != nil {
			symbols = append(symbols, Symbol{
				Name:     match[1],
				Type:     "trigger",
				Line:     lineNum,
				Parent:   match[2],
				Exported: true,
			})
			continue
		}

		// CREATE TYPE AS ENUM
		if match := typePattern.FindStringSubmatch(trimmed); match != nil {
			symbols = append(symbols, Symbol{
				Name:     match[1],
				Type:     "enum",
				Line:     lineNum,
				Exported: true,
			})
			continue
		}

		// CREATE SEQUENCE
		if match := seqPattern.FindStringSubmatch(trimmed); match != nil {
			symbols = append(symbols, Symbol{
				Name:     match[1],
				Type:     "constant",
				Line:     lineNum,
				Exported: true,
			})
			continue
		}
	}

	// Also extract columns (need to re-scan within table blocks)
	symbols = append(symbols, extractSQLColumns(string(content), lines)...)

	return symbols, true, nil
}

func extractSQLColumns(content string, lines []string) []Symbol {
	var symbols []Symbol
	
	// Find CREATE TABLE blocks and extract columns with defaults
	tableBlockPattern := regexp.MustCompile(`(?is)CREATE\s+TABLE\s+(\w+)\s*\((.*?)\);`)
	columnPattern := regexp.MustCompile(`(?i)(\w+)\s+\w+.*?DEFAULT\s+([^,\n]+)`)

	for _, match := range tableBlockPattern.FindAllStringSubmatch(content, -1) {
		tableName := match[1]
		tableBody := match[2]
		
		// Find line number for table
		tableLine := 1
		for i, line := range lines {
			if strings.Contains(strings.ToLower(line), "create table "+strings.ToLower(tableName)) {
				tableLine = i + 1
				break
			}
		}

		// Extract columns with defaults
		for _, colMatch := range columnPattern.FindAllStringSubmatch(tableBody, -1) {
			colName := colMatch[1]
			defaultVal := strings.TrimSpace(colMatch[2])
			
			// Find approximate line for column
			colLine := tableLine
			for i := tableLine; i < len(lines) && i < tableLine+20; i++ {
				if strings.Contains(strings.ToLower(lines[i]), strings.ToLower(colName)) {
					colLine = i + 1
					break
				}
			}

			symbols = append(symbols, Symbol{
				Name:       tableName + "." + colName,
				Type:       "column",
				Line:       colLine,
				Parent:     tableName,
				Value:      cleanSQLDefault(defaultVal),
				DocComment: "DEFAULT " + defaultVal,
				Exported:   true,
			})
		}
	}

	return symbols
}

func cleanSQLDefault(value string) string {
	// Remove trailing comma or parenthesis
	value = strings.TrimRight(value, ",)")
	value = strings.TrimSpace(value)
	// Remove quotes
	if strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
		return value[1 : len(value)-1]
	}
	return value
}

// ============================================================
// Cypher Parser
// ============================================================

func parseCypher(filePath string) ([]Symbol, bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, true, err
	}

	var symbols []Symbol
	lines := strings.Split(string(content), "\n")
	
	// Track seen items to avoid duplicates
	seenLabels := make(map[string]bool)
	seenRelTypes := make(map[string]bool)

	constraintPattern := regexp.MustCompile(`(?i)CREATE\s+CONSTRAINT\s+(\w+).*FOR\s+\(\w+:(\w+)\)`)
	indexPattern := regexp.MustCompile(`(?i)CREATE\s+INDEX\s+(\w+).*FOR\s+\(\w+:(\w+)\)`)
	createNodePattern := regexp.MustCompile(`CREATE\s+\(\w+:(\w+)`)
	matchNodePattern := regexp.MustCompile(`MATCH\s+.*\(\w+:(\w+)`)
	relPattern := regexp.MustCompile(`-\[:(\w+)`)

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		// Skip comments
		if strings.HasPrefix(trimmed, "//") {
			continue
		}

		// Constraints
		if match := constraintPattern.FindStringSubmatch(trimmed); match != nil {
			constraintName := match[1]
			labelName := match[2]
			
			symbols = append(symbols, Symbol{
				Name:     constraintName,
				Type:     "constraint",
				Line:     lineNum,
				Parent:   labelName,
				Exported: true,
			})
			
			if !seenLabels[labelName] {
				seenLabels[labelName] = true
				symbols = append(symbols, Symbol{
					Name:     labelName,
					Type:     "label",
					Line:     lineNum,
					Exported: true,
				})
			}
			continue
		}

		// Indexes
		if match := indexPattern.FindStringSubmatch(trimmed); match != nil {
			indexName := match[1]
			labelName := match[2]
			
			symbols = append(symbols, Symbol{
				Name:     indexName,
				Type:     "index",
				Line:     lineNum,
				Parent:   labelName,
				Exported: true,
			})
			
			if !seenLabels[labelName] {
				seenLabels[labelName] = true
				symbols = append(symbols, Symbol{
					Name:     labelName,
					Type:     "label",
					Line:     lineNum,
					Exported: true,
				})
			}
			continue
		}

		// CREATE (n:Label {...})
		for _, match := range createNodePattern.FindAllStringSubmatch(trimmed, -1) {
			labelName := match[1]
			if !seenLabels[labelName] {
				seenLabels[labelName] = true
				symbols = append(symbols, Symbol{
					Name:     labelName,
					Type:     "label",
					Line:     lineNum,
					Exported: true,
				})
			}
		}

		// Relationship types -[:TYPE]-
		for _, match := range relPattern.FindAllStringSubmatch(trimmed, -1) {
			relType := match[1]
			if !seenRelTypes[relType] {
				seenRelTypes[relType] = true
				symbols = append(symbols, Symbol{
					Name:     relType,
					Type:     "relationship_type",
					Line:     lineNum,
					Exported: true,
				})
			}
		}
	}

	return symbols, true, nil
}
