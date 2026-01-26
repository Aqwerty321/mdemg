// Command plugin-validate validates MDEMG plugins for correctness.
//
// Usage:
//
//	go run ./cmd/plugin-validate --plugin=./plugins/my-plugin
//
// This tool performs comprehensive validation of plugin manifests,
// proto compliance, health checks, and full lifecycle testing.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"mdemg/internal/plugins"
)

// Color codes for terminal output
var (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorBold   = "\033[1m"
)

func main() {
	pluginPath := flag.String("plugin", "", "Path to plugin directory (required)")
	socketPath := flag.String("socket", "", "Path to running plugin socket (for health/lifecycle validation)")
	manifestOnly := flag.Bool("manifest-only", false, "Only validate manifest.json")
	protoOnly := flag.Bool("proto-only", false, "Only validate proto compliance")
	healthOnly := flag.Bool("health-only", false, "Only validate health check (requires --socket)")
	lifecycleOnly := flag.Bool("lifecycle-only", false, "Only validate lifecycle (requires --socket)")
	jsonOutput := flag.Bool("json", false, "Output results as JSON")
	verbose := flag.Bool("verbose", false, "Show detailed output")
	noColor := flag.Bool("no-color", false, "Disable colored output")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: plugin-validate [options]\n\n")
		fmt.Fprintf(os.Stderr, "Validates MDEMG plugins for correctness.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  # Validate all aspects of a plugin\n")
		fmt.Fprintf(os.Stderr, "  plugin-validate --plugin=./plugins/my-plugin\n\n")
		fmt.Fprintf(os.Stderr, "  # Validate only the manifest\n")
		fmt.Fprintf(os.Stderr, "  plugin-validate --plugin=./plugins/my-plugin --manifest-only\n\n")
		fmt.Fprintf(os.Stderr, "  # Validate a running plugin's health\n")
		fmt.Fprintf(os.Stderr, "  plugin-validate --socket=/var/run/mdemg/mdemg-my-plugin.sock --health-only\n\n")
		fmt.Fprintf(os.Stderr, "  # Output results as JSON\n")
		fmt.Fprintf(os.Stderr, "  plugin-validate --plugin=./plugins/my-plugin --json\n")
	}

	flag.Parse()

	// Disable colors if requested or if not a terminal
	if *noColor || os.Getenv("NO_COLOR") != "" || !isTerminal() {
		disableColors()
	}

	// Validate flags
	if *pluginPath == "" && *socketPath == "" {
		fmt.Fprintln(os.Stderr, "Error: --plugin or --socket is required")
		flag.Usage()
		os.Exit(1)
	}

	if (*healthOnly || *lifecycleOnly) && *socketPath == "" {
		fmt.Fprintln(os.Stderr, "Error: --socket is required for health or lifecycle validation")
		os.Exit(1)
	}

	// Resolve paths
	if *pluginPath != "" {
		absPath, err := filepath.Abs(*pluginPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error resolving path: %v\n", err)
			os.Exit(1)
		}
		*pluginPath = absPath
	}

	// Run validations
	var exitCode int
	result := &plugins.PluginValidation{
		PluginPath: *pluginPath,
		Valid:      true,
	}

	if *socketPath != "" {
		result.PluginPath = *socketPath
	}

	// Manifest validation
	if *pluginPath != "" && !*protoOnly && !*healthOnly && !*lifecycleOnly {
		if !*jsonOutput {
			printSection("Manifest Validation")
		}

		manifestResult, err := plugins.ValidateManifest(*pluginPath)
		if err != nil {
			if *jsonOutput {
				outputJSONError(err)
			} else {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			}
			os.Exit(1)
		}

		result.Manifest = manifestResult
		if !manifestResult.Valid {
			result.Valid = false
		}

		if !*jsonOutput {
			printManifestResult(manifestResult, *verbose)
		}

		if *manifestOnly {
			if *jsonOutput {
				outputJSON(result)
			}
			if !result.Valid {
				exitCode = 1
			}
			os.Exit(exitCode)
		}
	}

	// Proto compliance validation
	if *pluginPath != "" && !*manifestOnly && !*healthOnly && !*lifecycleOnly {
		if result.Manifest == nil || result.Manifest.Manifest == nil {
			// Need to read manifest first
			manifestResult, err := plugins.ValidateManifest(*pluginPath)
			if err != nil || !manifestResult.Valid {
				if !*jsonOutput {
					fmt.Fprintln(os.Stderr, "Cannot validate proto compliance without valid manifest")
				}
				os.Exit(1)
			}
			result.Manifest = manifestResult
		}

		if !*jsonOutput {
			printSection("Proto Compliance Validation")
		}

		binaryPath := filepath.Join(*pluginPath, result.Manifest.Manifest.Binary)
		protoResult, err := plugins.ValidateProtoCompliance(binaryPath, result.Manifest.Manifest.Type)
		if err != nil {
			if *jsonOutput {
				outputJSONError(err)
			} else {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			}
			os.Exit(1)
		}

		result.Proto = protoResult
		if !protoResult.Valid {
			result.Valid = false
		}

		if !*jsonOutput {
			printProtoResult(protoResult, *verbose)
		}

		if *protoOnly {
			if *jsonOutput {
				outputJSON(result)
			}
			if !result.Valid {
				exitCode = 1
			}
			os.Exit(exitCode)
		}
	}

	// Health check validation
	if *socketPath != "" && (*healthOnly || (!*manifestOnly && !*protoOnly && !*lifecycleOnly)) {
		if !*jsonOutput {
			printSection("Health Check Validation")
		}

		healthResult, err := plugins.ValidateHealthCheck(*socketPath)
		if err != nil {
			if *jsonOutput {
				outputJSONError(err)
			} else {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			}
			os.Exit(1)
		}

		result.Health = healthResult
		if !healthResult.Valid {
			result.Valid = false
		}

		if !*jsonOutput {
			printHealthResult(healthResult, *verbose)
		}

		if *healthOnly {
			if *jsonOutput {
				outputJSON(result)
			}
			if !result.Valid {
				exitCode = 1
			}
			os.Exit(exitCode)
		}
	}

	// Lifecycle validation
	if *socketPath != "" && (*lifecycleOnly || (!*manifestOnly && !*protoOnly && !*healthOnly)) {
		if !*jsonOutput {
			printSection("Lifecycle Validation")
		}

		lifecycleResult, err := plugins.ValidateLifecycle(*socketPath)
		if err != nil {
			if *jsonOutput {
				outputJSONError(err)
			} else {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			}
			os.Exit(1)
		}

		result.Lifecycle = lifecycleResult
		if !lifecycleResult.Valid {
			result.Valid = false
		}

		if !*jsonOutput {
			printLifecycleResult(lifecycleResult, *verbose)
		}
	}

	// Output final results
	if *jsonOutput {
		outputJSON(result)
	} else {
		printSummary(result)
	}

	if !result.Valid {
		exitCode = 1
	}
	os.Exit(exitCode)
}

func printSection(title string) {
	fmt.Printf("\n%s%s=== %s ===%s\n\n", colorBold, colorBlue, title, colorReset)
}

func printManifestResult(result *plugins.ManifestValidation, verbose bool) {
	if result.Valid {
		fmt.Printf("%s[PASS]%s Manifest validation passed\n", colorGreen, colorReset)
	} else {
		fmt.Printf("%s[FAIL]%s Manifest validation failed\n", colorRed, colorReset)
	}

	if result.Manifest != nil && verbose {
		fmt.Printf("\n  Plugin ID:    %s\n", result.Manifest.ID)
		fmt.Printf("  Plugin Name:  %s\n", result.Manifest.Name)
		fmt.Printf("  Version:      %s\n", result.Manifest.Version)
		fmt.Printf("  Type:         %s\n", result.Manifest.Type)
		fmt.Printf("  Binary:       %s\n", result.Manifest.Binary)
	}

	if len(result.Errors) > 0 {
		fmt.Printf("\n  %sErrors:%s\n", colorRed, colorReset)
		for _, e := range result.Errors {
			fmt.Printf("    - %s\n", e)
		}
	}

	if len(result.Warnings) > 0 {
		fmt.Printf("\n  %sWarnings:%s\n", colorYellow, colorReset)
		for _, w := range result.Warnings {
			fmt.Printf("    - %s\n", w)
		}
	}
}

func printProtoResult(result *plugins.ProtoValidation, verbose bool) {
	if result.Valid {
		fmt.Printf("%s[PASS]%s Proto compliance validation passed\n", colorGreen, colorReset)
	} else {
		fmt.Printf("%s[FAIL]%s Proto compliance validation failed\n", colorRed, colorReset)
	}

	if verbose {
		if len(result.ServicesRegistered) > 0 {
			fmt.Printf("\n  Services Registered:\n")
			for _, s := range result.ServicesRegistered {
				fmt.Printf("    - %s\n", s)
			}
		}

		if len(result.RPCsImplemented) > 0 {
			fmt.Printf("\n  RPCs Implemented:\n")
			for _, r := range result.RPCsImplemented {
				fmt.Printf("    - %s%s%s\n", colorGreen, r, colorReset)
			}
		}
	}

	if len(result.RPCsMissing) > 0 {
		fmt.Printf("\n  %sRPCs Missing:%s\n", colorRed, colorReset)
		for _, r := range result.RPCsMissing {
			fmt.Printf("    - %s\n", r)
		}
	}

	if len(result.Errors) > 0 {
		fmt.Printf("\n  %sErrors:%s\n", colorRed, colorReset)
		for _, e := range result.Errors {
			fmt.Printf("    - %s\n", e)
		}
	}

	if len(result.Warnings) > 0 {
		fmt.Printf("\n  %sWarnings:%s\n", colorYellow, colorReset)
		for _, w := range result.Warnings {
			fmt.Printf("    - %s\n", w)
		}
	}
}

func printHealthResult(result *plugins.HealthValidation, verbose bool) {
	if result.Valid {
		fmt.Printf("%s[PASS]%s Health check validation passed\n", colorGreen, colorReset)
	} else {
		fmt.Printf("%s[FAIL]%s Health check validation failed\n", colorRed, colorReset)
	}

	if verbose || !result.Healthy {
		fmt.Printf("\n  Healthy:       %v\n", result.Healthy)
		fmt.Printf("  Status:        %s\n", result.Status)
		fmt.Printf("  Response Time: %dms\n", result.ResponseTimeMs)

		if len(result.Metrics) > 0 {
			fmt.Printf("\n  Metrics:\n")
			for k, v := range result.Metrics {
				fmt.Printf("    %s: %s\n", k, v)
			}
		}
	}

	if len(result.Errors) > 0 {
		fmt.Printf("\n  %sErrors:%s\n", colorRed, colorReset)
		for _, e := range result.Errors {
			fmt.Printf("    - %s\n", e)
		}
	}

	if len(result.Warnings) > 0 {
		fmt.Printf("\n  %sWarnings:%s\n", colorYellow, colorReset)
		for _, w := range result.Warnings {
			fmt.Printf("    - %s\n", w)
		}
	}
}

func printLifecycleResult(result *plugins.LifecycleValidation, verbose bool) {
	if result.Valid {
		fmt.Printf("%s[PASS]%s Lifecycle validation passed\n", colorGreen, colorReset)
	} else {
		fmt.Printf("%s[FAIL]%s Lifecycle validation failed\n", colorRed, colorReset)
	}

	if verbose {
		fmt.Printf("\n  Module ID:      %s\n", result.ModuleID)
		fmt.Printf("  Module Version: %s\n", result.ModuleVersion)
		fmt.Printf("  Total Duration: %dms\n", result.TotalDurationMs)
	}

	// Always show step results
	fmt.Printf("\n  Lifecycle Steps:\n")
	printStepResult("Handshake", result.HandshakeOK)
	printStepResult("Health Check", result.HealthOK)
	printStepResult("Shutdown", result.ShutdownOK)

	if len(result.Errors) > 0 {
		fmt.Printf("\n  %sErrors:%s\n", colorRed, colorReset)
		for _, e := range result.Errors {
			fmt.Printf("    - %s\n", e)
		}
	}

	if len(result.Warnings) > 0 {
		fmt.Printf("\n  %sWarnings:%s\n", colorYellow, colorReset)
		for _, w := range result.Warnings {
			fmt.Printf("    - %s\n", w)
		}
	}
}

func printStepResult(name string, ok bool) {
	if ok {
		fmt.Printf("    %s[OK]%s %s\n", colorGreen, colorReset, name)
	} else {
		fmt.Printf("    %s[FAIL]%s %s\n", colorRed, colorReset, name)
	}
}

func printSummary(result *plugins.PluginValidation) {
	fmt.Printf("\n%s=== Summary ===%s\n\n", colorBold, colorReset)

	var passed, failed, warnings int

	if result.Manifest != nil {
		if result.Manifest.Valid {
			passed++
		} else {
			failed++
		}
		warnings += len(result.Manifest.Warnings)
	}

	if result.Proto != nil {
		if result.Proto.Valid {
			passed++
		} else {
			failed++
		}
		warnings += len(result.Proto.Warnings)
	}

	if result.Health != nil {
		if result.Health.Valid {
			passed++
		} else {
			failed++
		}
		warnings += len(result.Health.Warnings)
	}

	if result.Lifecycle != nil {
		if result.Lifecycle.Valid {
			passed++
		} else {
			failed++
		}
		warnings += len(result.Lifecycle.Warnings)
	}

	fmt.Printf("  Validations Passed:  %s%d%s\n", colorGreen, passed, colorReset)
	fmt.Printf("  Validations Failed:  %s%d%s\n", colorRed, failed, colorReset)
	fmt.Printf("  Total Warnings:      %s%d%s\n", colorYellow, warnings, colorReset)

	fmt.Println()
	if result.Valid {
		fmt.Printf("%s%sPlugin validation PASSED%s\n", colorBold, colorGreen, colorReset)
	} else {
		fmt.Printf("%s%sPlugin validation FAILED%s\n", colorBold, colorRed, colorReset)
	}
	fmt.Println()
}

func outputJSON(result *plugins.PluginValidation) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(result)
}

func outputJSONError(err error) {
	result := map[string]interface{}{
		"valid": false,
		"error": err.Error(),
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(result)
}

func isTerminal() bool {
	fileInfo, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}

func disableColors() {
	colorReset = ""
	colorRed = ""
	colorGreen = ""
	colorYellow = ""
	colorBlue = ""
	colorBold = ""
}
