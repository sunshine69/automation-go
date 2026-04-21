package lib

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	u "github.com/sunshine69/golang-tools/utils"
	"gopkg.in/yaml.v3"
)

// Host: DNS-safe, no children, no groupSet field
type Host struct {
	Name   string         `json:"name"`
	Groups []string       `json:"groups,omitempty"`
	Vars   map[string]any `json:"vars,omitempty"`
}

// Group
type Group struct {
	Name     string         `json:"name"`
	Hosts    []string       `json:"hosts,omitempty"`
	Children []string       `json:"children,omitempty"`
	Vars     map[string]any `json:"vars,omitempty"`
}

// Inventory
type Inventory struct {
	All          *Group            `json:"all,omitempty"`
	Groups       map[string]*Group `json:"groups"`
	Hosts        map[string]*Host  `json:"hosts"`
	GroupOrder   []string          `json:"group_order,omitempty"`
	InventoryDir string
}

func NewInventory(inventoryDir string) *Inventory {
	return &Inventory{
		Groups:       make(map[string]*Group),
		Hosts:        make(map[string]*Host),
		InventoryDir: inventoryDir,
	}
}

// parseInlineVarsSmart parses inline host vars like "key1=val1 key2='val 2' key3=\"val 3\""
func parseInlineVarsSmart(line string) map[string]any {
	vars := make(map[string]any)
	// Regex to match: key = value (value may be quoted or unquoted)
	re := regexp.MustCompile(`(\w+)=((?:"[^"]*"|'[^']*'|[^\s]+)+)`)
	matches := re.FindAllStringSubmatch(line, -1)
	for _, m := range matches {
		if len(m) < 3 {
			continue
		}
		key := m[1]
		rawVal := m[2]
		val := parseValue(rawVal)
		vars[key] = val
	}
	return vars
}

// unquote
func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// parseInlineVars: ["k1=v1", "k2='v2'"] → map[string]any
// Converts each value to appropriate type (int, bool, string, etc.)
func parseInlineVars(tokens []string) map[string]any {
	vars := make(map[string]any)
	for _, tok := range tokens {
		parts := strings.SplitN(tok, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		valRaw := parts[1]
		val := parseValue(valRaw)
		vars[key] = val
	}
	return vars
}

// DNS validator (RFC 1123-friendly: alphanumeric, . _ -)
var validHostname = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

func hostValid(name string) bool {
	if name == "" || len(name) > 253 {
		return false
	}
	return validHostname.MatchString(name)
}

// AddHost — validates hostname strictly
func (inv *Inventory) AddHost(hostname string) *Host {
	if !hostValid(hostname) {
		fmt.Fprintf(os.Stderr, "Warning: invalid hostname '%s'\n", hostname)
		return nil
	}
	if _, ok := inv.Hosts[hostname]; !ok {
		inv.Hosts[hostname] = &Host{
			Name:   hostname,
			Groups: []string{},
			Vars:   make(map[string]any),
		}
	}
	return inv.Hosts[hostname]
}

// AddGroup
func (inv *Inventory) AddGroup(name string) *Group {
	if name == "" {
		return nil
	}
	name = strings.TrimSpace(name)
	if _, ok := inv.Groups[name]; !ok {
		inv.GroupOrder = append(inv.GroupOrder, name)
		inv.Groups[name] = &Group{
			Name:     name,
			Hosts:    []string{},
			Children: []string{},
			Vars:     make(map[string]any),
		}
	}
	return inv.Groups[name]
}

// parseSectionHeader: only parses [name], [name:vars], [name:children]
func parseSectionHeader(line string) (name, sectionType string) {
	if !strings.HasPrefix(line, "[") || !strings.HasSuffix(line, "]") {
		return "", ""
	}
	content := strings.TrimSpace(line[1 : len(line)-1])
	if content == "" {
		return "", ""
	}

	// Only zero or one ':' — and only in valid contexts
	if strings.Count(content, ":") > 1 {
		return "", ""
	}

	if idx := strings.Index(content, ":"); idx != -1 {
		name = content[:idx]
		sectionType = content[idx+1:]
		if sectionType != "vars" && sectionType != "children" {
			return "", ""
		}
	} else {
		name = content
		sectionType = ""
	}

	if !hostValid(name) {
		return "", ""
	}

	return name, sectionType
}

// parseLineForHost: only hostname (first token), rejects ':', spaces, invalid chars
func parseLineForHost(line string) (string, error) {
	line = strings.TrimSpace(line)
	if line == "" || line[0] == '#' || line[0] == ';' {
		return "", fmt.Errorf("empty or comment")
	}

	// Reject if line contains ':' — but only for host lines
	if strings.Contains(line, ":") {
		return "", fmt.Errorf("invalid hostname '%s': contains ':'", line)
	}

	tokens := strings.Fields(line)
	if len(tokens) == 0 {
		return "", fmt.Errorf("no hostname")
	}
	hostname := tokens[0]

	if !hostValid(hostname) {
		return "", fmt.Errorf("invalid hostname '%s'", hostname)
	}
	return hostname, nil
}

// Helper to guess type (string, int, float, bool, or raw string)
func parseValue(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	// Try bool
	if b, err := strconv.ParseBool(s); err == nil {
		return b
	}
	// Try int
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i
	}
	// Try float
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	// Otherwise, unquote string
	return unquote(s)
}

// ParseInventory parses inventory from either a file path (string) or io.Reader
func ParseInventory(src any, inv *Inventory) error {
	switch v := src.(type) {
	case string:
		file, err := os.Open(v)
		if err != nil {
			return fmt.Errorf("failed to open %s: %w", v, err)
		}
		defer file.Close()
		return ParseInventoryReader(file, inv)
	case io.Reader:
		return ParseInventoryReader(v, inv)
	default:
		return fmt.Errorf("unsupported source type %T", src)
	}
}

// internal helper for scanning
func ParseInventoryReader(r io.Reader, inv *Inventory) error {
	var currentGroup *Group
	var currentSectionType string // "", "vars", or "children"

	scanner := bufio.NewScanner(r)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			name, sectionType := parseSectionHeader(line)
			if name == "" {
				continue
			}

			switch sectionType {
			case "":
				currentGroup = inv.AddGroup(name)
				currentSectionType = ""
			case "vars", "children":
				currentGroup = inv.AddGroup(name)
				currentSectionType = sectionType
				currentGroup = nil // disable host parsing
				continue
			default:
				currentGroup = nil
				continue
			}
			continue
		}

		// Skip host lines in [name:vars] and [name:children]
		if currentSectionType == "vars" || currentSectionType == "children" {
			continue
		}

		if currentGroup == nil {
			fmt.Fprintf(os.Stderr, "Warning (source:%d): host line '%s' outside any group\n", lineNum, line)
			continue
		}

		hostname, err := parseLineForHost(line)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning (source:%d): %v\n", lineNum, err)
			continue
		}

		host := inv.AddHost(hostname)
		if host == nil {
			continue
		}

		// Ensure "all" group exists
		allInserted := false
		for _, g := range host.Groups {
			if g == "all" {
				allInserted = true
				break
			}
		}
		if !allInserted {
			host.Groups = append([]string{"all"}, host.Groups...)
		}

		// Add host to current group (avoid duplicates)
		if !containsStr(currentGroup.Hosts, hostname) {
			currentGroup.Hosts = append(currentGroup.Hosts, hostname)
		}
		if !containsStr(host.Groups, currentGroup.Name) {
			host.Groups = append(host.Groups, currentGroup.Name)
		}

		// Inline vars
		tokens := strings.Fields(line)
		if len(tokens) > 1 {
			lineAfterHost := strings.Join(tokens[1:], " ")
			vars := parseInlineVarsSmart(lineAfterHost)
			if host.Vars == nil {
				host.Vars = make(map[string]any)
			}
			for k, v := range vars {
				host.Vars[k] = v
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanner error: %w", err)
	}
	return nil
}

// ParseInventoryDir: parses all .ini and extensionless files in directory
func ParseInventoryDir(invDir string) (*Inventory, error) {
	inv := NewInventory(invDir)

	entries, err := os.ReadDir(invDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", invDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := filepath.Ext(name)
		// Accept .ini and files without extension
		if ext != ".ini" && ext != "" {
			continue
		}

		fullPath := filepath.Join(invDir, name)
		err := ParseInventory(fullPath, inv)
		if err != nil {
			return nil, fmt.Errorf("error parsing file %s: %w", fullPath, err)
		}
	}

	// Build implicit `all` group
	inv.All = &Group{
		Name:     "all",
		Hosts:    []string{},
		Children: []string{},
		Vars:     make(map[string]any),
	}
	for hostName := range inv.Hosts {
		inv.All.Hosts = append(inv.All.Hosts, hostName)
	}

	sortGroupOrder(inv)
	FinalizeInventory(inv)
	return inv, nil
}

// FinalizeInventory: sort groups per host by priority (simples before composites)
func FinalizeInventory(inv *Inventory) {
	for _, host := range inv.Hosts {
		simples, composites := []string{}, []string{}
		for _, g := range host.Groups {
			if strings.Contains(g, "_") {
				composites = append(composites, g)
			} else {
				simples = append(simples, g)
			}
		}
		sort.Strings(simples)
		sort.Strings(composites)
		host.Groups = append(simples, composites...)
	}
}

func sortGroupOrder(inv *Inventory) {
	simples, composites := []string{}, []string{}
	for _, g := range inv.GroupOrder {
		if strings.Contains(g, "_") {
			composites = append(composites, g)
		} else {
			simples = append(simples, g)
		}
	}
	sort.Strings(simples)
	sort.Strings(composites)
	inv.GroupOrder = append(simples, composites...)
}

func containsStr(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// ParseInventoryVars: parses [name:vars] sections for groups/hosts
func (inv *Inventory) ParseInventoryVars(arg any) error {
	switch v := arg.(type) {
	case string:
		// Parse from directory
		return inv.parseInventoryVarsDir(v)
	case io.Reader:
		// Parse from reader
		return inv.ParseInventoryVarsReader(v)
	default:
		return fmt.Errorf("unsupported argument type: %T", arg)
	}
}

// parseInventoryVarsDir: helper to parse vars from directory
func (inv *Inventory) parseInventoryVarsDir(invDir string) error {
	entries, err := os.ReadDir(invDir)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %w", invDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := filepath.Ext(name)
		if ext != ".ini" && ext != "" {
			continue
		}

		fullPath := filepath.Join(invDir, name)
		file, err := os.Open(fullPath)
		if err != nil {
			continue
		}
		defer file.Close()

		if err := inv.ParseInventoryVarsReader(file); err != nil {
			return err
		}
	}
	return nil
}

// ParseInventoryVarsReader: parses inventory vars from an io.Reader
func (inv *Inventory) ParseInventoryVarsReader(reader io.Reader) error {
	scanner := bufio.NewScanner(reader)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}

		// Only process [name:vars] sections
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			name, sectionType := parseSectionHeader(line)
			if sectionType != "vars" || name == "" {
				continue
			}

			// Found [name:vars] — now parse its content
			// Check if group exists
			if g, ok := inv.Groups[name]; ok {
				g.Vars = make(map[string]any)
				for scanner.Scan() {
					line = strings.TrimSpace(scanner.Text())
					if strings.HasPrefix(line, "[") || line == "" {
						break
					}
					if idx := strings.Index(line, "="); idx != -1 {
						key := strings.TrimSpace(line[:idx])
						val := unquote(line[idx+1:])
						g.Vars[key] = val
					}
				}
			} else if h, ok := inv.Hosts[name]; ok {
				h.Vars = make(map[string]any)
				for scanner.Scan() {
					line = strings.TrimSpace(scanner.Text())
					if strings.HasPrefix(line, "[") || line == "" {
						break
					}
					if idx := strings.Index(line, "="); idx != -1 {
						key := strings.TrimSpace(line[:idx])
						val := unquote(line[idx+1:])
						h.Vars[key] = val
					}
				}
			} else {
				fmt.Fprintf(os.Stderr, "Warning: [%s:vars] references unknown group or host '%s'\n", name, name)
			}
		}
	}
	return scanner.Err()
}

type GeneratorConfig struct {
	Plugin string              `yaml:"plugin"`
	Hosts  HostConfig          `yaml:"hosts"`
	Layers map[string][]string `yaml:"layers"`
}

type HostConfig struct {
	Name    string            `yaml:"name"`
	Vars    map[string]string `yaml:"vars"`
	Parents []GroupConfig     `yaml:"parents"`
}

type GroupConfig struct {
	Name    string            `yaml:"name"`
	Vars    map[string]string `yaml:"vars"`
	Parents []GroupConfig     `yaml:"parents"`
}

// FlattenVar recursively resolves all template variables in a string
// until no more {{ }} patterns remain
// visited map has key "cached" -> map[string]any that use to cache between recursion and
// "visited" -> bool to mark key visited or not, this will be deleted after the recursion complete
func FlattenVar(key string, data map[string]any, visited map[string]any) (any, error) {
	if visited["visited"] == nil {
		visited["visited"] = make(map[string]bool)
	}
	// Check for circular dependencies
	if visited["visited"].(map[string]bool)[key] {
		return "", fmt.Errorf("circular dependency detected for key: %s", key)
	}

	// Get the value for this key
	val, ok := data[key]
	if !ok {
		return "", fmt.Errorf("key not found: %s", key)
	}

	// Caching what can be cached
	var vaultPtn, varRe, findCurly *regexp.Regexp
	var vaultPass string

	if visited["cached"] == nil {
		visited["cached"] = make(map[string]any)
	}
	if vaultPtn, ok = visited["cached"].(map[string]any)["regex"].(*regexp.Regexp); !ok {
		vaultPtn = regexp.MustCompile(`<vault>(.*?)</vault>`)
		visited["cached"].(map[string]any)["regex"] = vaultPtn
		vaultPass = os.Getenv("VAULT_PASSWORD")
		visited["cached"].(map[string]any)["passwd"] = vaultPass
		varRe = regexp.MustCompile(`\{\{\s*(\w+)(?:\s|\}|\.|\|)`)
		visited["cached"].(map[string]any)["regexvarRe"] = varRe
		findCurly = regexp.MustCompile(`\{\{|\}\}`)
		visited["cached"].(map[string]any)["regexvarFindCurly"] = findCurly
	} else {
		vaultPass = visited["cached"].(map[string]any)["passwd"].(string)
		vaultPtn = visited["cached"].(map[string]any)["regex"].(*regexp.Regexp)
		// Regular expression to find any variable references in the data map
		varRe = visited["cached"].(map[string]any)["regexvarRe"].(*regexp.Regexp)
		findCurly = visited["cached"].(map[string]any)["regexvarFindCurly"].(*regexp.Regexp)
	}

	// Convert to string TODO maybe parse to some object?
	var decodedVal any
	switch v := val.(type) {
	case string: // Vars coming from aini lib are map[string]string thus it will fall into this case
		tempVal := v
		// Decrypt vault data if any
		if vaultPass != "" {
			match := vaultPtn.FindStringSubmatch(tempVal)
			if len(match) > 1 {
				if decrypted, err := u.Decrypt(match[1], vaultPass, u.DefaultEncryptionConfig()); err == nil {
					tempVal = vaultPtn.ReplaceAllString(tempVal, decrypted)
				}
			}
		}
		// Keep resolving until no more {{ }} patterns exist
		maxIterations := 100 // Prevent infinite loops
		for i := 0; i < maxIterations; i++ {
			// Check if there are any {{ or }} left (simple check for Jinja2 templates)
			if !findCurly.MatchString(tempVal) {
				break
			}
			// Find all referenced variables and flatten them first
			matches := varRe.FindAllStringSubmatch(tempVal, -1)
			for _, match := range matches {
				if len(match) > 1 {
					refKey := match[1]
					// Recursively flatten the referenced variable
					if _, exists := data[refKey]; exists {
						flattened, err := FlattenVar(refKey, data, visited)
						if err != nil {
							return "", err
						}
						// Update the data map with flattened value
						data[refKey] = flattened
					}
				}
			}
			// Now template the current string
			tempVal = TemplateString(tempVal, data)
		}
		// More detection - parser?
		tempVal_, err := parseDynamicValue(tempVal)
		if err != nil {
			decodedVal = tempVal
		} else {
			decodedVal = tempVal_
		}

	default: // Normally override vars later on fall into this
		decodedVal = v
	}
	// Mark this key as being processed
	visited["visited"].(map[string]bool)[key] = true
	defer delete(visited["visited"].(map[string]bool), key)

	return decodedVal, nil
}

// FlattenAllVars flattens all variables in the data map
func FlattenAllVars(data map[string]any) (map[string]any, error) {
	result := make(map[string]any)

	visited := make(map[string]any) // Shared this to utilize cache. Each key will be deleetd after each call FlattenVar anyway
	for key := range data {
		flattened, err := FlattenVar(key, data, visited)
		if err != nil {
			return nil, err
		}
		result[key] = flattened
	}

	return result, nil
}

// Parse all vars in correct order to preserve vars priorities
func (inv *Inventory) ParseAllInventoryVars() {
	u.CheckErr(inv.ParseGroupVars(inv.InventoryDir), "")
	inv.MergeVars()
	u.CheckErr(inv.ParseInventoryVars(inv.InventoryDir), "")
	inv.MergeVarNotOverriding()
	u.CheckErr(inv.ParseHostVars(inv.InventoryDir), "")
	u.CheckErr(inv.FlattenAllVars(), "")
}

// ParseGroupVars reads YAML files named <groupname>.yml from group_vars/
// If invDir is empty it uses the current from Inventory, otherwise it will use the dir and update the
// current Inventory Dir
func (inv *Inventory) ParseGroupVars(invDir string) error {
	if invDir == "" {
		invDir = inv.InventoryDir
	} else {
		inv.InventoryDir = invDir
	}
	groupVarsDir := filepath.Join(invDir, "group_vars")
	if _, err := os.Stat(groupVarsDir); os.IsNotExist(err) {
		return nil
	}
	for _, ext := range []string{".yml", ".yaml"} {
		allPath := filepath.Join(groupVarsDir, "all"+ext)
		if _, err := os.Stat(allPath); err == nil {
			if vars, err := parseYAMLFile(allPath); err == nil {
				g, ok := inv.Groups["all"]
				if !ok {
					g = &Group{Name: "all"}
					g.Vars = make(map[string]any)
					g.Vars["inventory_dir"] = inv.InventoryDir // BUG if we have the generator with vars defined it lost
					inv.Groups["all"] = g
				}
				for k, v := range vars {
					g.Vars[k] = v
				}
			}
			break
		}
	}
	for groupName := range inv.Groups {
		// Try .yml first, then .yaml
		for _, ext := range []string{".yml", ".yaml"} {
			filePath := filepath.Join(groupVarsDir, groupName+ext)
			if _, err := os.Stat(filePath); err == nil {
				// File found
				vars, err := parseYAMLFile(filePath)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
					continue
				}

				// Ensure group.Vars is initialized
				if inv.Groups[groupName].Vars == nil {
					inv.Groups[groupName].Vars = make(map[string]any)
				}

				// Merge (convert to string)
				for k, v := range vars {
					inv.Groups[groupName].Vars[k] = v
				}
				break // success — stop trying other extensions
			}
		}
	}
	return nil
}

// ParseHostVars reads YAML files named <hostname>.yml from host_vars/
func (inv *Inventory) ParseHostVars(invDir string) error {
	if invDir == "" {
		invDir = inv.InventoryDir
	} else {
		inv.InventoryDir = invDir
	}

	hostVarsDir := filepath.Join(invDir, "host_vars")
	if _, err := os.Stat(hostVarsDir); os.IsNotExist(err) {
		return nil
	}

	for hostName := range inv.Hosts {
		for _, ext := range []string{".yml", ".yaml"} {
			filePath := filepath.Join(hostVarsDir, hostName+ext)
			if _, err := os.Stat(filePath); err == nil {
				vars, err := parseYAMLFile(filePath)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
					continue
				}

				if inv.Hosts[hostName].Vars == nil {
					inv.Hosts[hostName].Vars = make(map[string]any)
				}

				for k, v := range vars {
					inv.Hosts[hostName].Vars[k] = fmt.Sprintf("%v", v)
				}
				break
			}
		}
	}
	return nil
}

// MergeVars applies group vars to each host in order of host.Groups.
// Later groups (higher index in host.Groups) override earlier ones.
// Host vars are *not* touched here — they will be applied later via ParseHostVars.
func (inv *Inventory) MergeVars() {
	for hostName, host := range inv.Hosts {
		// Preserve host's existing inline vars (e.g., from host line like "web1 key=value")
		// We'll re-apply these later, but for MergeVars we *don't* use them as base.
		// Instead: group vars completely overwrite the host.Vars map (if any)
		// But we want to *start empty* — group vars build up the final host.Vars.
		hostVars := make(map[string]any)

		// Apply group vars in host.Groups order (preserve group ordering)
		for _, groupName := range host.Groups {
			g, ok := inv.Groups[groupName]
			if !ok || len(g.Vars) == 0 {
				continue
			}

			// Override: later groups overwrite earlier groups
			for k, v := range g.Vars {
				hostVars[k] = v
			}
		}

		// Assign the merged result back
		if len(hostVars) == 0 {
			host.Vars = nil // clean up empty maps
		} else {
			host.Vars = hostVars
		}
		inv.Hosts[hostName] = host
	}
}

// MergeVarNotOverriding applies group vars to each host in order of host.Groups.
// If a host already has a variable (from inline vars or prior merges), it will NOT be overridden by group vars.
func (inv *Inventory) MergeVarNotOverriding() {
	for hostName, host := range inv.Hosts {
		// Start with a copy of the host's existing Vars (to preserve them)
		hostVars := make(map[string]any)
		for k, v := range host.Vars {
			hostVars[k] = v
		}

		// Apply group vars, but only if the host doesn't already have them
		for _, groupName := range host.Groups {
			g, ok := inv.Groups[groupName]
			if !ok || len(g.Vars) == 0 {
				continue
			}

			// Skip if the key already exists in hostVars
			for k, v := range g.Vars {
				if _, exists := hostVars[k]; !exists {
					hostVars[k] = v
				}
			}
		}

		// Assign the merged result back
		if len(hostVars) == 0 {
			host.Vars = nil // clean up empty maps
		} else {
			host.Vars = hostVars
		}
		inv.Hosts[hostName] = host
	}
}

func parseYAMLFile(filePath string) (map[string]interface{}, error) {
	data, err := os.ReadFile(filePath) // <-- replaces ioutil.ReadFile
	if err != nil {
		return nil, err
	}

	var vars map[string]interface{}
	if err := yaml.Unmarshal(data, &vars); err != nil {
		return nil, fmt.Errorf("yaml parse error in %s: %w", filePath, err)
	}
	return vars, nil
}

func parseDynamicValue(val string) (interface{}, error) {
	var detected interface{}

	// Attempt to unmarshal the string as JSON
	err := json.Unmarshal([]byte(val), &detected)

	if err != nil {
		// Not valid JSON; return original string as a fallback
		return val, nil
	}

	// Return the converted type (could be float64, bool, []interface{}, or map[string]interface{})
	return detected, nil
}

// FlattenAllVars flattens Jinja2 templates and expressions in each host's Vars
func (inv *Inventory) FlattenAllVars() error {
	for _, host := range inv.Hosts {
		if host.Vars == nil || len(host.Vars) == 0 {
			continue
		}

		flat, err := FlattenAllVars(host.Vars)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: flatten vars for host %s failed: %v\n", host.Name, err)
			continue
		}

		// Replace with flattened map (preserves same keys, updated values)
		host.Vars = flat
	}
	return nil
}

// The Yaml Generator -> INI conversion section. This is not realy readable, subject to review and redo it
// Layer expansion (cartesian product)
// This turns:
// ops: [update]
// pkg: [letsencrypt]
// env: [dev, uat, prod]
// into:
//
//	[]map[string]any{
//		{"ops":"update","pkg":"letsencrypt","env":"dev"},
//		{"ops":"update","pkg":"letsencrypt","env":"uat"},
//		{"ops":"update","pkg":"letsencrypt","env":"prod"},
//	}
func ExpandLayers(layers map[string][]string) []map[string]any {
	keys := make([]string, 0, len(layers))
	for k := range layers {
		keys = append(keys, k)
	}

	var res []map[string]any

	var walk func(int, map[string]any)
	walk = func(i int, cur map[string]any) {
		if i == len(keys) {
			// println("[DEBUG]", u.JsonDump(cur, ""), "[END]")
			// m := map[string]any{}
			// for k, v := range cur {
			// 	m[k] = v
			// }
			m := maps.Clone(cur)
			res = append(res, m)
			return
		}

		key := keys[i]
		for _, val := range layers[key] {
			cur[key] = val
			walk(i+1, cur)
		}
	}

	walk(0, map[string]any{})
	return res
}

type IniInventory struct {
	Groups        map[string][]string          // group -> hosts
	GroupVars     map[string]map[string]string // group -> vars
	GroupChildren map[string]map[string]bool   // parent -> child groups
}

func ensureGroupChildren(m map[string]map[string]bool, parent string) {
	if _, ok := m[parent]; !ok {
		m[parent] = map[string]bool{}
	}
}

func ensureGroup(m map[string][]string, name string) {
	if _, ok := m[name]; !ok {
		m[name] = []string{}
	}
}

func ensureGroupVars(m map[string]map[string]string, name string) {
	if _, ok := m[name]; !ok {
		m[name] = map[string]string{}
	}
}

func GenerateIniFromConfig(cfg *GeneratorConfig) string {
	inv := &IniInventory{
		Groups:        map[string][]string{},
		GroupVars:     map[string]map[string]string{},
		GroupChildren: map[string]map[string]bool{},
	}

	objects := ExpandLayers(cfg.Layers)

	for _, ctx := range objects {
		host := TemplateString(cfg.Hosts.Name, ctx)

		ensureGroup(inv.Groups, "all")
		inv.Groups["all"] = append(inv.Groups["all"], host)

		for _, p := range cfg.Hosts.Parents {
			processGroupINI(inv, p, ctx, host, true)
		}

	}

	if len(cfg.Hosts.Vars) > 0 {
		ensureGroupVars(inv.GroupVars, "all")
		for k, v := range cfg.Hosts.Vars {
			inv.GroupVars["all"][k] = v
		}
	}
	// fmt.Printf("DEBUG GroupChildren = %s\n", u.JsonDump(inv.GroupChildren, ""))
	return renderIni(inv)
}

func renderIni(inv *IniInventory) string {
	var b strings.Builder

	for g, hosts := range inv.Groups {
		b.WriteString("[" + g + "]\n")
		for _, h := range hosts {
			b.WriteString(h + "\n")
		}
		b.WriteString("\n")
	}

	for parent, children := range inv.GroupChildren {
		b.WriteString("[" + parent + ":children]\n")
		for c := range maps.Keys(children) {
			b.WriteString(c + "\n")
		}
		// render [parent:children]
	}
	for g, vars := range inv.GroupVars {
		b.WriteString("[" + g + ":vars]\n")
		for k, v := range vars {
			b.WriteString(fmt.Sprintf("%s=%s\n", k, v))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func processGroupINI(
	inv *IniInventory,
	cfg GroupConfig,
	ctx map[string]any,
	host string,
	withHost bool,
) {
	groupName := TemplateString(cfg.Name, ctx)
	if groupName == "" {
		return
	}

	// ensure group exists
	ensureGroup(inv.Groups, groupName)

	// only leaf groups get hosts
	if withHost {
		inv.Groups[groupName] = append(inv.Groups[groupName], host)
	}

	// Process current group's vars first
	ensureGroupVars(inv.GroupVars, groupName)
	for k, tmpl := range cfg.Vars {
		v := TemplateString(tmpl, ctx)
		inv.GroupVars[groupName][k] = v
		// Update context with the resolved value for nested processing
		ctx[k] = v
	}

	// parents → children relationship
	for _, p := range cfg.Parents {
		parentName := TemplateString(p.Name, ctx)
		if parentName == "" {
			continue
		}
		ensureGroupChildren(inv.GroupChildren, parentName)
		inv.GroupChildren[parentName][groupName] = true

		// ensure parent exists
		ensureGroup(inv.Groups, parentName)

		// Create a copy of context for parent processing to avoid polluting current ctx
		parentCtx := maps.Clone(ctx)

		// Merge parent vars into parentCtx for nested processing
		for k, tmpl := range p.Vars {
			v := TemplateString(tmpl, ctx)
			parentCtx[k] = v
		}

		// Recurse upward WITHOUT hosts
		processGroupINI(inv, p, parentCtx, host, false)
	}
}

// ReadFirstLevelFiles reads the first level of a directory and returns only the files.
func ReadFirstLevelFiles(dirPath string) ([]fs.DirEntry, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	var files []fs.DirEntry
	for _, entry := range entries {
		// Check if the entry is a file
		if !entry.IsDir() {
			files = append(files, entry)
		}
	}

	return files, nil
}

// Parse all type inventory fiels in the dir, currently support generator and ini format.
func ParseInventoryDirAll(inventoryDir string) *Inventory {
	invFiles := u.Must(ReadFirstLevelFiles(inventoryDir))
	readers := []io.Reader{}

	for _, invF := range invFiles {
		filePath := filepath.Join(inventoryDir, invF.Name())
		ext := filepath.Ext(filePath)

		switch ext {
		case ".yaml", ".yml":
			println("Processing YAML file: " + filePath)
			invConfig := GeneratorConfig{}
			u.CheckErr(yaml.Unmarshal(u.Must(os.ReadFile(filePath)), &invConfig), "")
			iniContent := GenerateIniFromConfig(&invConfig)
			readers = append(readers, strings.NewReader(iniContent))
		case ".ini":
			// Let ParseInventoryDir handle .ini files separately
			// (we'll pass only non-ini readers to ParseInventory)
			continue
		}
	}

	// Start with base INI parsing (e.g., from inventoryDir/*.ini)
	inv := u.Must(ParseInventoryDir(inventoryDir))
	// Then layer on YAML→INI content
	if len(readers) > 0 {
		u.CheckErr(ParseInventory(io.MultiReader(readers...), inv), "")
	}
	return inv
}

// MatchHost returns all hostnames matching the given regex pattern
func (inv *Inventory) MatchHost(pattern string) []string {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return []string{}
	}

	var result []string
	for hostname := range inv.Hosts {
		if re.MatchString(hostname) {
			result = append(result, hostname)
		}
	}
	sort.Strings(result)
	return result
}

// MatchGroup returns all group names matching the given regex pattern
func (inv *Inventory) MatchGroup(pattern string) []string {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return []string{}
	}

	var result []string
	for groupName := range inv.Groups {
		if re.MatchString(groupName) {
			result = append(result, groupName)
		}
	}
	sort.Strings(result)
	return result
}
