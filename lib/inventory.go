package lib

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/relex/aini"
	u "github.com/sunshine69/golang-tools/utils"
	"gopkg.in/yaml.v3"
)

type Inventory struct {
	Groups map[string]*Group
}

type Group struct {
	Hosts map[string]*Host
	Vars  map[string]any
}

type Host struct {
	Vars map[string]any
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

// Layer expansion (cartesian product)
// This turns:
// ops: [update]
// pkg: [letsencrypt]
// env: [dev, uat, prod]
// into:
// []map[string]any{
// 	{"ops":"update","pkg":"letsencrypt","env":"dev"},
// 	{"ops":"update","pkg":"letsencrypt","env":"uat"},
// 	{"ops":"update","pkg":"letsencrypt","env":"prod"},
// }

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

	// group vars
	for k, tmpl := range cfg.Vars {
		v := TemplateString(tmpl, ctx)
		ensureGroupVars(inv.GroupVars, groupName)
		inv.GroupVars[groupName][k] = v
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

		// recurse upward WITHOUT hosts
		processGroupINI(inv, p, ctx, host, false)
	}
}

func ParseInventoryGenerator(inventoryFile string) *aini.InventoryData {
	datab := u.Must(os.ReadFile(inventoryFile))
	invConfig := GeneratorConfig{}
	u.CheckErr(yaml.Unmarshal(datab, &invConfig), "")
	iniText := GenerateIniFromConfig(&invConfig)
	inventory := u.Must(aini.ParseString(iniText))
	inventory.AddVars(filepath.Dir(inventoryFile))
	return inventory
}

// Load all inventory files at the first level. If file does not have extention or having .ini then treat it as ini format.
// .yaml file will be treated as generator format. We dont support anything else as of now
// Inventory will be combined. AddVars also called but var wont be flattern yet, you need to do it manually
func ParseInventoryDir(inventoryDir string) *aini.InventoryData {
	invFiles := u.Must(ReadFirstLevelFiles(inventoryDir))
	builder := strings.Builder{}

	for _, invF := range invFiles {
		file_path := filepath.Join(inventoryDir, invF.Name())
		ext := filepath.Ext(file_path)
		println("Read file " + file_path)
		if ext == "" || ext == ".ini" {
			println("Read ini file " + file_path)
			builder.Write(u.Must(os.ReadFile(file_path)))
			builder.WriteString("\n")
			continue
		}
		if ext == ".yaml" || ext == ".yml" {
			println("Read yaml file " + file_path)
			invConfig := GeneratorConfig{}
			u.CheckErr(yaml.Unmarshal(u.Must(os.ReadFile(file_path)), &invConfig), "")
			builder.WriteString(GenerateIniFromConfig(&invConfig))
			builder.WriteString("\n")
		}
	}
	Inventory := u.Must(aini.ParseString(builder.String()))
	Inventory.AddVars(inventoryDir)
	return Inventory
}

// Load inventory and return command line vars in Vars. Also populate global vars.
// Per host will get its own vars later
func LoadInventory(InventoryDir, HostsPattern string, extraVar u.ArrayFlags) (Inventory *aini.InventoryData, MatchedHostsMap map[string]*aini.Host, HostList []string, extraVars map[string]any) {
	extraVars = make(map[string]any, 0)
	if _, ok := extraVars["inventory_dir"]; ok {
		return // Not reload it again
	}
	println("[INFO] InventoryPath: " + InventoryDir)
	Inventory = ParseInventoryDir(InventoryDir)
	MatchedHostsMap = u.Must(Inventory.MatchHosts(HostsPattern))
	HostList = u.MapKeysToSlice(MatchedHostsMap)
	// Populate some default inventory vars. The specific host before use will update this Vars with ansible vars and flattern them
	extraVars["inventory_dir"] = InventoryDir
	extraVars["playbook_dir"] = u.Must(os.Getwd())

	// Loads command line vars
	for _, item := range extraVar {
		_tmp := strings.Split(item, "=")
		key, val := strings.TrimSpace(_tmp[0]), strings.TrimSpace(_tmp[1])
		println("Adding var from command line - " + key + "=" + val)
		extraVars[key] = val
	}
	return
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
