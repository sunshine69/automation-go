package lib

import (
	"bytes"
	b64 "encoding/base64"
	"fmt"
	"io/fs"
	"sort"
	"time"

	"github.com/mitsuhiko/minijinja/minijinja-go/v2/filters"
	"github.com/mitsuhiko/minijinja/minijinja-go/v2/syntax"
	"github.com/mitsuhiko/minijinja/minijinja-go/v2/value"

	"os"
	"path/filepath"
	"regexp"
	"strings"

	mj "github.com/mitsuhiko/minijinja/minijinja-go/v2"
	u "github.com/sunshine69/golang-tools/utils"
	"gopkg.in/yaml.v3"
)

func parseJinja2Config(firstLine, prefix string) (foundConfig bool, whc syntax.WhitespaceConfig, cfg syntax.SyntaxConfig) {
	whc = syntax.DefaultWhitespace()
	cfg = syntax.DefaultSyntax()

	for _, _token := range strings.Split(strings.TrimPrefix(firstLine, prefix), ",") {
		_token0 := strings.TrimSpace(_token)
		_data := strings.Split(_token0, ":")
		switch _data[0] {
		case "variable_start_string":
			foundConfig = true
			cfg.VarStart = strings.Trim(strings.Trim(_data[1], `'`), `"`)
		case "variable_end_string":
			foundConfig = true
			cfg.VarEnd = strings.Trim(strings.Trim(_data[1], `'`), `"`)
		case "trim_blocks":
			foundConfig = true
			whc.TrimBlocks = _data[1] == "True"
		case "lstrip_blocks":
			foundConfig = true
			whc.LstripBlocks = _data[1] == "True"
		}
	}
	return
}

func NewJinjaEnvironment(whc *syntax.WhitespaceConfig, cfg *syntax.SyntaxConfig) *mj.Environment {
	env := mj.NewEnvironment()
	if whc != nil {
		env.SetWhitespace(*whc)
	}
	if cfg != nil {
		env.SetSyntax(*cfg)
	}
	// =========================================
	// Custom Tests
	// =========================================
	env.AddTest("startswith", testStartWith)
	// A test for checking if a number is in a range
	env.AddTest("between", testBetween)

	// =========================================
	// Custom Functions
	// =========================================
	// A function that returns the current time
	env.AddFunction("now", filterNow)
	env.AddFilter("reverse_str", filterReverseStr)
	// A filter with arguments that wraps text in a tag
	env.AddFilter("wrap", filterWrap)
	env.AddFilter("regex_replace", filterFuncRegexReplace)
	env.AddFilter("regex_search", filterFuncRegexSearch)
	env.AddFilter("to_yaml", filterFuncToYaml)
	env.AddFilter("to_json", filters.FilterTojson) // Just a rename - the original on is tojson
	env.AddFilter("b64encode", filterFuncB64Encode)
	env.AddFilter("b64decode", filterFuncB64Decode)
	env.AddFilter("contains", filterContainsAll)
	env.AddFilter("contains_any", filterContainsAny)
	env.AddFilter("keys", FilterKeys)
	env.AddFilter("indent", filterIndent)
	return env
}

// FilterKeys returns a list of keys from a map.
//
// The keys are sorted alphabetically.
//
// Example:
//
//	env := minijinja.NewEnvironment()
//	env.AddFilter("keys", FilterKeys)
//
// Template usage:
//
//	{{ my_dict|keys }}
func FilterKeys(_ mj.FilterState, val value.Value, _ []value.Value, _ map[string]value.Value) (value.Value, error) {
	if m, ok := val.AsMap(); ok {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		result := make([]value.Value, len(keys))
		for i, k := range keys {
			result[i] = value.FromString(k)
		}
		return value.FromSlice(result), nil
	}
	return value.FromSlice(nil), nil
}

// FilterValues returns a list of values from a map.
//
// The values are returned in the same order as the sorted keys.
//
// Example:
//
//	env := minijinja.NewEnvironment()
//	env.AddFilter("values", FilterValues)
//
// Template usage:
//
//	{{ my_dict|values }}
func FilterValues(_ mj.FilterState, val value.Value, _ []value.Value, _ map[string]value.Value) (value.Value, error) {
	if m, ok := val.AsMap(); ok {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		result := make([]value.Value, len(keys))
		for i, k := range keys {
			result[i] = m[k]
		}
		return value.FromSlice(result), nil
	}
	return value.FromSlice(nil), nil
}

func filterReverseStr(state mj.FilterState, val value.Value, args []value.Value, kwargs map[string]value.Value) (value.Value, error) {
	s, ok := val.AsString()
	if !ok {
		return value.Undefined(), fmt.Errorf("reverse_str expects a string")
	}
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return value.FromString(string(runes)), nil
}

func filterWrap(state mj.FilterState, val value.Value, args []value.Value, kwargs map[string]value.Value) (value.Value, error) {
	s, ok := val.AsString()
	if !ok {
		return value.Undefined(), fmt.Errorf("wrap expects a string")
	}

	tag := "span"
	if len(args) > 0 {
		if t, ok := args[0].AsString(); ok {
			tag = t
		}
	}
	// Check for class kwarg
	class := ""
	if c, ok := kwargs["class"]; ok {
		if cs, ok := c.AsString(); ok {
			class = fmt.Sprintf(` class="%s"`, cs)
		}
	}
	result := fmt.Sprintf("<%s%s>%s</%s>", tag, class, s, tag)
	// Return as safe string since we're generating HTML
	return value.FromSafeString(result), nil
}
func filterNow(state *mj.State, args []value.Value, kwargs map[string]value.Value) (value.Value, error) {
	format := time.RFC3339
	if len(args) > 0 {
		if f, ok := args[0].AsString(); ok {
			format = f
		}
	}
	return value.FromString(time.Now().Format(format)), nil
}

func testStartWith(state mj.TestState, val value.Value, args []value.Value) (bool, error) {
	s, ok := val.AsString()
	if !ok {
		return false, nil
	}
	if len(args) == 0 {
		return false, fmt.Errorf("startswith requires a prefix argument")
	}
	prefix, ok := args[0].AsString()
	if !ok {
		return false, nil
	}
	return strings.HasPrefix(s, prefix), nil
}

func testBetween(state mj.TestState, val value.Value, args []value.Value) (bool, error) {
	n, ok := val.AsInt()
	if !ok {
		return false, nil
	}
	if len(args) < 2 {
		return false, fmt.Errorf("between requires min and max arguments")
	}
	min, _ := args[0].AsInt()
	max, _ := args[1].AsInt()
	return n >= min && n <= max, nil
}

func filterIndent(_ mj.FilterState, val value.Value, args []value.Value, kwargs map[string]value.Value) (value.Value, error) {
	s, ok := val.AsString()
	if !ok {
		return val, nil
	}

	width := 4
	if len(args) > 0 {
		if w, ok := args[0].AsInt(); ok {
			width = int(w)
		}
	}

	for key, val := range kwargs {
		switch key {
		case "width":
			if valI, ok := val.AsInt(); ok {
				width = int(valI)
			}
		default:
			return value.Undefined(), fmt.Errorf("unspoorted keyword arguments %s", key)
		}
	}

	first := false
	if len(args) > 1 {
		if b, ok := args[1].AsBool(); ok {
			first = b
		}
	}

	blank := false
	if len(args) > 2 {
		if b, ok := args[2].AsBool(); ok {
			blank = b
		}
	}

	indent := strings.Repeat(" ", width)
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if i == 0 && !first {
			continue
		}
		if line == "" && !blank {
			continue
		}
		lines[i] = indent + line
	}
	return value.FromString(strings.Join(lines, "\n")), nil
}

// var filterKeys mj.FilterFunc = func(state mj.FilterState, val value.Value, args []value.Value, kwargs map[string]value.Value) (value.Value, error) {
// 	// native := ValueToNative(val)

// 	// out := u.MapKeysToSlice(native.(map[string]any))
// 	// return value.FromSlice(out), nil

// }

var ptnPerlCapture *regexp.Regexp = regexp.MustCompile(`\\(\d+)`)

func convertPerlCapPattern(input string) string {
	// Define the regular expression to match the pattern \d+
	// Replace the matched pattern with $ and the digits
	result := ptnPerlCapture.ReplaceAllStringFunc(input, func(match string) string {
		return "$" + match[1:]
	})

	return result
}

func filterFuncRegexReplace(state mj.FilterState, val value.Value, args []value.Value, kwargs map[string]value.Value) (value.Value, error) {
	s, ok := val.AsString()
	if !ok {
		debugStr := ""
		debug, ok := kwargs["DEBUG"]
		if ok {
			debugStr, _ = debug.AsString()
		}
		return value.Undefined(), fmt.Errorf("regex_replace expects a string. I got %s. debugstr is: %s", u.JsonDump(val, ""), debugStr)
	}
	// 2 args is mandate, option arg count default val is nil. Backslash \ need to escape like \\ so template compile works
	// p := params.Expect(2, []*exec.KwArg{{Name: "count", Default: nil}})
	// if p.IsError() {
	// 	return exec.AsValue(errors.Wrap(p, "Wrong signature for 'regex_replace'"))
	// }
	if len(args) < 2 {
		return value.Undefined(), fmt.Errorf("regex_replace expects two args: pattern, replacement")
	}
	pattern, _ := args[0].AsString()
	new, _ := args[1].AsString()
	new = convertPerlCapPattern(new)

	ptn := regexp.MustCompile(pattern)

	output := ptn.ReplaceAllString(s, new)
	return value.FromString(output), nil
}

func filterFuncRegexSearch(state mj.FilterState, val value.Value, args []value.Value, kwargs map[string]value.Value) (value.Value, error) {
	input, ok := val.AsString()
	if !ok {
		return value.Undefined(), fmt.Errorf("regex_search expects a string")
	}
	// Simulate the ansible regex_search filter. Map to golang FindString and FindStringSubMatch
	if len(args) == 0 {
		return value.Undefined(), fmt.Errorf("regex_search expects 1 arg: pattern")
	}
	ptnStr, _ := args[0].AsString()
	ptn := regexp.MustCompile(ptnStr)

	out := ptn.FindStringSubmatch(input)
	if len(out) == 1 { // return a match
		return value.FromString(out[0]), nil
	} // If there is capture then return a list of captures (submatch). Ignoring all args
	if len(out) > 1 {
		return value.FromSlice(u.SliceWalk(out[1:], func(s string) *value.Value { v := value.FromString(s); return &v })), nil
	}
	return value.FromString(""), nil
}

func filterFuncToYaml(state mj.FilterState, val value.Value, args []value.Value, kwargs map[string]value.Value) (value.Value, error) {
	input := ValueToNative(val)

	var indent int64 = 2
	if p, ok := kwargs["indent"]; ok {
		if _indent, ok := p.AsInt(); ok {
			indent = _indent
		}
	}

	var buf bytes.Buffer
	// Create a new YAML encoder with a custom indentation level
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(int(indent))

	if err := encoder.Encode(input); err != nil {
		panic(fmt.Sprintf("Error encoding YAML: %s\n", err))
	}
	return value.FromString(buf.String()), nil
}

func filterFuncB64Encode(state mj.FilterState, val value.Value, args []value.Value, kwargs map[string]value.Value) (value.Value, error) {
	input, ok := val.AsString()
	if !ok {
		return value.Undefined(), fmt.Errorf("b64encode expects a string")
	}
	// wrap is unsupported in golang, try to implement it later on
	o := b64.StdEncoding.EncodeToString([]byte(input))
	return value.FromString(o), nil
}

func filterFuncB64Decode(state mj.FilterState, val value.Value, args []value.Value, kwargs map[string]value.Value) (value.Value, error) {
	input, ok := val.AsString()
	if !ok {
		return value.Undefined(), fmt.Errorf("b64decode expects a string")
	}
	o := u.Must(b64.StdEncoding.DecodeString(input))
	return value.FromBytes(o), nil
}

func filterContainsAll(state mj.FilterState, val value.Value, args []value.Value, kwargs map[string]value.Value) (value.Value, error) {
	mainList, ok := val.AsSlice()
	if !ok {
		return value.Undefined(), fmt.Errorf("contains_all expects a list")
	}
	// The sub_list passed as an argument: {{ main_list | contains_all(sub_list) }}
	subList, ok := args[0].AsSlice()
	if !ok {
		return value.Undefined(), fmt.Errorf("contains_all expects second list")
	}
	mainList1 := u.SliceWalk(mainList, func(item value.Value) *string { s, _ := item.AsString(); return &s })
	subList1 := u.SliceWalk(subList, func(item value.Value) *string { s, _ := item.AsString(); return &s })
	return value.FromBool(u.SliceContainsItems(mainList1, subList1)), nil
}

func filterContainsAny(state mj.FilterState, val value.Value, args []value.Value, kwargs map[string]value.Value) (value.Value, error) {
	mainList, ok := val.AsSlice()
	if !ok {
		return value.Undefined(), fmt.Errorf("contains_all expects a list")
	}
	// The sub_list passed as an argument: {{ main_list | contains_all(sub_list) }}
	subList, ok := args[0].AsSlice()
	if !ok {
		return value.Undefined(), fmt.Errorf("contains_all expects second list")
	}
	mainList1 := u.SliceWalk(mainList, func(item value.Value) *string { s, _ := item.AsString(); return &s })
	subList1 := u.SliceWalk(subList, func(item value.Value) *string { s, _ := item.AsString(); return &s })
	mainListMap := u.SliceToMap(mainList1)

	for _, item := range subList1 {
		if _, ok := mainListMap[item]; ok {
			return value.FromBool(true), nil
		}
	}
	return value.FromBool(false), nil
}

// InspectTemplateString inspects the first line of a string to determine
// if it contains Jinja2-specific configuration directives.
//
// If a configuration line is found, it returns the remaining source text
// and the parsed WhitespaceConfig and SyntaxConfig. If no configuration
// is found, it returns the original text and defaults.
// Caller can always use the remainText as source of templateString
//
// The function uses the environment variable JINJA2_CONFIG_LINE_PREFIX
// (default: "#jinja2:") to identify configuration lines.
func InspectTemplateString(text string) (foundConfig bool, remainText string, whc syntax.WhitespaceConfig, cfg syntax.SyntaxConfig) {
	firstLine, newSrc := u.SplitFirstLine(text)
	if newSrc == "" {
		return false, text, syntax.DefaultWhitespace(), syntax.DefaultSyntax()
	}
	prefix := u.Getenv("JINJA2_CONFIG_LINE_PREFIX", `#jinja2:`)
	_foundConfig, whc, cfg := parseJinja2Config(firstLine, prefix)
	if !_foundConfig {
		newSrc = text
		foundConfig = false
		whc.LstripBlocks = true
		whc.TrimBlocks = true
	}
	return foundConfig, newSrc, whc, cfg
}

// InspectTemplateFile reads the content of a file and uses InspectTemplateString
// to determine if it contains Jinja2 configuration directives.
//
// Returns the processing flag, remaining text, and the parsed whitespace and
// syntax configurations based on the file's contents.
// Caller can always use the remainText as source of templateString
func InspectTemplateFile(filePath string) (foundConfig bool, remainText string, whc syntax.WhitespaceConfig, cfg syntax.SyntaxConfig) {
	data := string(u.Must(os.ReadFile(filePath)))
	return InspectTemplateString(data)
}

// Template a file using template string and convert windows new line to unix. This is
// work around the gonja2 windows new line problem
func TemplateFile(src, dest string, data map[string]any, fileMode os.FileMode) {
	if fileMode == 0 {
		fileMode = 0o777
	}
	srcB := u.Must(os.ReadFile(src))
	_, remain, whc, cfg := InspectTemplateString(string(srcB))
	// println("FILE: " + src)
	// println("SOURCE: " + string(srcB))
	// println("REMAIN: " + remain)
	env := NewJinjaEnvironment(&whc, &cfg)
	tmpl := u.Must(env.TemplateFromString(remain))
	destFile := u.Must(os.Create(dest))
	u.CheckErr(destFile.Chmod(fileMode), fmt.Sprintf("[ERROR] can not chmod %d for file %s\n", fileMode, dest))
	defer destFile.Close()
	u.CheckErr(tmpl.RenderToWrite(data, destFile), "TemplateFile - RenderToWrite")
	// out := u.Must(tmpl.Render(data))
	// u.Must(destFile.WriteString(out))
}

func parseConfigVarArgs(opt []string) (syntax.WhitespaceConfig, syntax.SyntaxConfig, map[string]string) {
	whc := syntax.DefaultWhitespace()
	whc.TrimBlocks = true
	whc.LstripBlocks = true
	cfg := syntax.DefaultSyntax()
	extraConfig := map[string]string{"replace_new_line": "True"}

	optLength := len(opt)
	if optLength >= 2 {
		for i := 0; i <= optLength-2; i += 2 {
			switch opt[i] {
			case "variable_start_string":
				cfg.VarStart = opt[i+1]
			case "variable_end_string":
				cfg.VarEnd = opt[i+1]
			case "trim_blocks":
				whc.TrimBlocks = opt[i+1] == "True"
			case "lstrip_blocks":
				whc.LstripBlocks = opt[i+1] == "True"
			case "comment_start_string":
				cfg.CommentStart = opt[i+1]
			case "comment_end_string":
				cfg.CommentEnd = opt[i+1]
			default:
				extraConfig[opt[i]] = opt[i+1]
			}
		}
	}
	return whc, cfg, extraConfig
}

// This func is suiatable to run on server as it wont crash but return err if tehre is err and have the most comprehensive options
//
// It also take the config options and do not parse the config line in the file so there is no intermedeate copy to temp file
// it a bit tiny faster esp when template large files > 10Mb for example
// Note there is known the windows new line problems if you this func. To replace_new_line set it to true
func TemplateFileWithConfig(src, dest string, data map[string]interface{}, fileMode os.FileMode, opt ...string) error {
	if fileMode == 0 {
		fileMode = 0o777
	}

	whc, cfg, extraConfig := parseConfigVarArgs(opt)

	if extraConfig["DEBUG"] == "True" {
		fmt.Fprintf(os.Stderr, "[DEBUG] %s\n", u.JsonDump(cfg, ""))
	}

	env := NewJinjaEnvironment(&whc, &cfg)

	dataS := string(u.Must(os.ReadFile(src)))
	tmpl, err := env.TemplateFromString(dataS)
	if err != nil {
		return err
	}
	destFile, err := os.Create(dest)
	if err != nil {
		return err
	}
	if err := u.CheckErrNonFatal(destFile.Chmod(fileMode), fmt.Sprintf("[ERROR] can not chmod %d for file %s\n", fileMode, dest)); err != nil {
		return err
	}
	defer destFile.Close()
	return tmpl.RenderToWrite(data, destFile)
}

// TemplateStringWithConfig renders a Jinja2 template string with the specified
// Whitespace and Syntax configurations.
//
// The opt argument allows passing whitespace and syntax options (e.g. "ignore:").
func TemplateStringWithConfig(srcString string, data map[string]interface{}, opt ...string) (string, error) {
	whc, cfg, _ := parseConfigVarArgs(opt)
	env := NewJinjaEnvironment(&whc, &cfg)
	tmpl, err := env.TemplateFromString(srcString)
	if err != nil {
		return "", err
	}
	return tmpl.Render(data)
}

// If a configuration line is found, it renders the template body with the
// specific settings. If no configuration is found, it renders the original
// string with default settings.
func TemplateString(srcString string, data map[string]interface{}) string {
	_, newSrc, whc, cfg := InspectTemplateString(srcString)
	if newSrc == "" {
		newSrc = srcString
	}
	env := NewJinjaEnvironment(&whc, &cfg)
	tmpl := u.Must(env.TemplateFromString(newSrc))
	return u.Must(tmpl.Render(data))
}

// TemplateDirTree read all templates files in the src directory and template to the target directory keeping the directory structure the same as source.
// Src and Target Path should be absolute path. They should not overlap to avoid recursive loop
func TemplateDirTree(srcDirpath, targetRoot string, tmplData map[string]interface{}) error {
	if isExist, err := u.FileExists(srcDirpath); !isExist || err != nil {
		panic("File " + srcDirpath + " does not exist\n")
	}
	os.Chdir(srcDirpath)
	filepath.Walk(".", func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			fmt.Printf("prevent panic by handling failure accessing a path %q: %v\n", srcDirpath, err)
			return err
		}
		if !info.IsDir() {
			srcFile, destFile := filepath.Join(srcDirpath, path), filepath.Join(targetRoot, path)
			fmt.Printf("Going to template file %s => %s\n", srcFile, destFile)
			destDir := filepath.Dir(destFile)
			u.CheckErr(os.MkdirAll(destDir, 0o777), "TemplateDirTree")
			TemplateFile(srcFile, destFile, tmplData, 0644)
		} else {
			u.CheckErr(os.MkdirAll(filepath.Join(targetRoot, path), 0755), "[ERROR] MkdirAll")
		}
		return nil
	})
	return nil
}

// LoadTemplatesInDirectory loads all template files from a directory and returns
// a map of template names (relative paths) to their parsed template objects.
//
// The function walks through the directory recursively, ignoring directories
// and processing only files. Template files are parsed using the default
// Jinja2 environment settings.
func LoadTemplatesInDirectory(dirPath string) (map[string]*mj.Template, error) {
	templates := make(map[string]*mj.Template)

	err := filepath.Walk(dirPath, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			// Get relative path from dirPath
			relPath, err := filepath.Rel(dirPath, path)
			if err != nil {
				return err
			}

			// Use InspectTemplateFile to get the parsed template
			_, remain, whc, cfg := InspectTemplateFile(path)

			// Create template with parsed settings
			env := NewJinjaEnvironment(&whc, &cfg)
			tmpl, err := env.TemplateFromString(remain)
			if err != nil {
				return fmt.Errorf("failed to parse template %s: %w", path, err)
			}

			templates[relPath] = tmpl
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return templates, nil
}

// ValueToNative converts a jinja2 value.Value type to a native Go interface{}.
//
// It handles conversion for primitive types (bool, int, float, string),
// slices (mapped to []interface{}), and maps (mapped to map[string]interface{}).
// For KindMap, it attempts to convert the keys to strings. If the value cannot be
// converted, it returns the raw string representation of the value.
func ValueToNative(v value.Value) interface{} {
	switch v.Kind() {
	case value.KindUndefined, value.KindNone:
		return nil
	case value.KindBool:
		b, _ := v.AsBool()
		return b
	case value.KindNumber:
		if i, ok := v.AsInt(); ok && v.IsActualInt() {
			return i
		}
		f, _ := v.AsFloat()
		return f
	case value.KindString:
		s, _ := v.AsString()
		return s
	case value.KindSeq:
		items, _ := v.AsSlice()
		result := make([]interface{}, len(items))
		for i, item := range items {
			result[i] = ValueToNative(item)
		}
		return result
	case value.KindMap:
		m, _ := v.AsMap()
		result := make(map[string]interface{})
		for k, val := range m {
			result[k] = ValueToNative(val)
		}
		return result
	default:
		if m, ok := v.AsMap(); ok {
			result := make(map[string]interface{})
			for k, val := range m {
				result[k] = ValueToNative(val)
			}
			return result
		}
		return v.String()
	}
}
