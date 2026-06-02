// AnalyzeYamlFile is the high-level orchestrator
package lib

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	u "github.com/sunshine69/golang-tools/utils"
	"go.yaml.in/yaml/v3"
)

type ValidationError struct {
	Line    int
	Message string
}

var (
	lineRegex = regexp.MustCompile(`line (\d+)`)
	// Detects lines ending in | or > which indicate multi-line block scalars
	yamlBlockScalarRegex = regexp.MustCompile(`[|>](\s*)$`)
)

// Validate yaml files. Optionally return the unmarshalled object if you pass yamlobj not nil
func ValidateYamlFile(yaml_file string) any {
	data, err := os.ReadFile(yaml_file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] %s\n", err.Error())
		return nil
	}
	var val any
	err = yaml.Unmarshal(data, &val)
	if err != nil {
		errMsg := fmt.Sprintf("[ERROR] file: %s - ", yaml_file)
		// STEP 1: Try Heuristic Fallback if the library fails
		heuristicErrs := runHeuristicScan(data)
		if len(heuristicErrs) > 0 {
			fmt.Fprintln(os.Stderr, errMsg+"\n"+u.JsonDump(heuristicErrs, ""))
			return nil
		}
		// STEP 2: If heuristics didn't catch it, return the raw library error
		// line := 0
		// match := lineRegex.FindStringSubmatch(err.Error())
		// if len(match) > 1 {
		// 	line, _ = strconv.Atoi(match[1])
		// }
		// panic(errMsg + "\n" + u.JsonDump([]ValidationError{{Line: line, Message: err.Error()}}, ""))

		var errs []ValidationError
		var root yaml.Node
		err = yaml.Unmarshal(data, &root)
		findVaultErrors(&root, &errs)
		if err != nil {
			fmt.Fprintln(os.Stderr, errMsg+err.Error())
			return nil
		}
	}
	return val
}

// Validate directory containing yaml files. Optionally return the unmarshalled object if you pass yamlobj not nil
func ValidateYamlDir(yaml_dir string) bool {
	// if yamlobj == nil {
	// 	t := map[string]interface{}{}
	// 	yamlobj = &t
	// }
	filepath.Walk(yaml_dir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		ext := filepath.Ext(info.Name())
		if ext == ".yaml" || ext == ".yml" {
			ValidateYamlFile(path)
		}
		return nil
	})
	return true
}

func AnalyzeYamlFile(path string) []ValidationError {
	data, err := os.ReadFile(path)
	if err != nil {
		return []ValidationError{{Line: 0, Message: fmt.Sprintf("Read error: %v", err)}}
	}
	return AnalyzeYamlContent(data)
}

// AnalyzeYamlContent separates the "Parsing" from "Reading" (making it testable)
func AnalyzeYamlContent(data []byte) []ValidationError {
	var root yaml.Node
	err := yaml.Unmarshal(data, &root)

	if err != nil {
		// STEP 1: Try Heuristic Fallback if the library fails
		heuristicErrs := runHeuristicScan(data)
		if len(heuristicErrs) > 0 {
			return heuristicErrs
		}

		// STEP 2: If heuristics didn't catch it, return the raw library error
		line := 0
		match := lineRegex.FindStringSubmatch(err.Error())
		if len(match) > 1 {
			line, _ = strconv.Atoi(match[1])
		}
		return []ValidationError{{Line: line, Message: err.Error()}}
	}

	// STEP 3: If syntax is valid, run logical (AST) validation
	var errs []ValidationError
	findVaultErrors(&root, &errs)
	return errs
}

// --- Heuristic Engine (The "Syntax Guard") ---

func runHeuristicScan(data []byte) []ValidationError {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	lineNum := 0
	inBlockScalar := false

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// 1. Handle Comments and Empty Lines
		if len(trimmed) == 0 || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// 2. Block Scalar State Machine
		// If the previous line was a block scalar indicator (| or >),
		// we skip syntax checking for this line.
		if inBlockScalar {
			// A block scalar ends when a line is empty or starts with a lower indentation
			// For a simple heuristic, we just check if the line is empty
			if len(trimmed) == 0 {
				inBlockScalar = false
			}
			continue
		}

		if yamlBlockScalarRegex.MatchString(trimmed) {
			inBlockScalar = true
			continue
		}

		// 3. Quote Balancing (with Escape awareness)
		if err := CheckUnbalancedQuotes(line, lineNum); err != nil {
			return []ValidationError{*err}
		}
	}
	return nil
}

func CheckUnbalancedQuotes(line string, lineNum int) *ValidationError {
	doubleCount := 0
	singleCount := 0
	escaped := false

	for _, r := range line {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if r == '"' {
			doubleCount++
		} else if r == '\'' {
			singleCount++
		}
	}

	if doubleCount%2 != 0 {
		return &ValidationError{Line: lineNum, Message: "Syntax Error: Unclosed double quote (\")"}
	}
	if singleCount%2 != 0 {
		return &ValidationError{Line: lineNum, Message: "Syntax Error: Unclosed single quote (')"}
	}
	return nil
}

// --- Logical Engine (The "AST Walker") ---

func findVaultErrors(node *yaml.Node, errs *[]ValidationError) {
	if node.Tag == "!vault" {
		validateVaultContent(node, errs)
	}
	for _, child := range node.Content {
		findVaultErrors(child, errs)
	}
}

func validateVaultContent(node *yaml.Node, errs *[]ValidationError) {
	val := strings.TrimSpace(node.Value)
	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, val)

	if !strings.Contains(cleaned, "ANSIBLE_VAULT") {
		*errs = append(*errs, ValidationError{Line: node.Line, Message: "Missing vault header ($ANSIBLE_VAULT;)"})
		return
	}
	// ... (Rest of your existing payload validation logic)
}
