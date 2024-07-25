package lib

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	b64 "encoding/base64"
	"fmt"
	"io"
	"io/fs"

	json "github.com/json-iterator/go"

	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nikolalohinski/gonja/v2"
	"github.com/nikolalohinski/gonja/v2/config"
	"github.com/nikolalohinski/gonja/v2/exec"
	"github.com/nikolalohinski/gonja/v2/loaders"
	"github.com/pkg/errors"
	u "github.com/sunshine69/golang-tools/utils"

	"gopkg.in/yaml.v3"
)

var CustomConfig = config.Config{
	BlockStartString:    "{%",
	BlockEndString:      "%}",
	VariableStartString: "{{",
	VariableEndString:   "}}",
	CommentStartString:  "{#",
	CommentEndString:    "#}",
	AutoEscape:          false,
	StrictUndefined:     false,
	TrimBlocks:          true,
	LeftStripBlocks:     true,
}

var ptnPerlCapture *regexp.Regexp = regexp.MustCompile(`\\(\d+)`)

func convertPerlCapPattern(input string) string {
	// Define the regular expression to match the pattern \d+
	// Replace the matched pattern with $ and the digits
	result := ptnPerlCapture.ReplaceAllStringFunc(input, func(match string) string {
		return "$" + match[1:]
	})

	return result
}

var filterFuncRegexReplace exec.FilterFunction = func(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	// 2 args is mandate, option arg count default val is nil. Backslash \ need to escape like \\ so template compile works
	// p := params.Expect(2, []*exec.KwArg{{Name: "count", Default: nil}})
	// if p.IsError() {
	// 	return exec.AsValue(errors.Wrap(p, "Wrong signature for 'regex_replace'"))
	// }
	pattern := params.Args[0].String()
	new := params.Args[1].String()
	new = convertPerlCapPattern(new)
	// count := params.KwArgs["count"]

	ptn := regexp.MustCompile(pattern)

	// if count.IsNil() {
	// 	return exec.AsValue(ptn.ReplaceAllString(in.String(), new))
	// }

	// counter := 0
	// output := ptn.ReplaceAllStringFunc(in.String(), func(value string) string {
	// 	if counter == count.Integer() {
	// 		return value
	// 	}
	// 	counter++
	// 	return ptn.ReplaceAllString(value, new)
	// })
	output := ptn.ReplaceAllString(in.String(), new)
	return exec.AsValue(output)
}

var filterFuncRegexSearch exec.FilterFunction = func(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	// Simulate the ansible regex_search filter. Map to golang FindString and FindStringSubMatch
	ptn := regexp.MustCompile(params.Args[0].String())
	input := in.String()

	out := ptn.FindStringSubmatch(input)
	if len(out) == 1 { // return a match
		fmt.Printf("DEBUG Matched 1 input '%s' - Ptn: '%s' Out: %v\n", in.String(), params.Args[0].String(), out[0])
		return exec.AsValue(out[0])
	} // If there is capture then return a list of captures (submatch). Ignoring all args
	if len(out) > 1 {
		return exec.AsValue(out[1:])
	}
	return exec.AsValue("")
}

var filterFuncToYaml exec.FilterFunction = func(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	p := params.Expect(0, []*exec.KwArg{{Name: "indent", Default: nil}})
	if p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'to_yaml'"))
	}
	indent := p.KwArgs["indent"]

	var buf bytes.Buffer
	// Create a new YAML encoder with a custom indentation level
	encoder := yaml.NewEncoder(&buf)

	if !indent.IsNil() {
		encoder.SetIndent(indent.Integer())
		fmt.Println("WARN indent not supported")
	}
	if err := encoder.Encode(in.ToGoSimpleType(true)); err != nil {
		panic(fmt.Sprintf("Error encoding YAML: %s\n", err))
	}

	return exec.AsValue(buf.String())
}

var filterFuncToJson exec.FilterFunction = func(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	// Done not mess around with trying to marshall error pipelines
	if in.IsError() {
		return in
	}

	// Monkey patching because arrays handling is broken
	if in.IsList() {
		inCast := make([]interface{}, in.Len())
		for index := range inCast {
			item := exec.ToValue(in.Index(index).Val)
			inCast[index] = item.Val.Interface()
		}
		in = exec.AsValue(inCast)
	}

	p := params.Expect(0, []*exec.KwArg{{Name: "indent", Default: nil}})
	if p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'tojson'"))
	}

	casted := in.ToGoSimpleType(true)
	if err, ok := casted.(error); ok {
		return exec.AsValue(err)
	}

	indent := p.KwArgs["indent"]
	var out string
	if indent.IsNil() {
		b, err := json.ConfigCompatibleWithStandardLibrary.Marshal(casted)
		if err != nil {
			return exec.AsValue(errors.Wrap(err, "Unable to marhsall to json"))
		}
		out = string(b)
	} else if indent.IsInteger() {
		b, err := json.ConfigCompatibleWithStandardLibrary.MarshalIndent(casted, "", strings.Repeat(" ", indent.Integer()))
		if err != nil {
			return exec.AsValue(errors.Wrap(err, "Unable to marhsall to json"))
		}
		out = string(b)
	} else {
		return exec.AsValue(errors.Errorf("Expected an integer for 'indent', got %s", indent.String()))
	}
	return exec.AsSafeValue(out)
}

var filterFuncB64Encode exec.FilterFunction = func(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	p := params.Expect(0, []*exec.KwArg{{Name: "wrap", Default: nil}})
	if p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'to_yaml'"))
	}
	// wrap is unsupported in golang, try to implement it later on
	o := b64.StdEncoding.EncodeToString([]byte(in.String()))
	return exec.AsValue(o)
}

var filterFuncB64Decode exec.FilterFunction = func(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	p := params.Expect(0, []*exec.KwArg{{Name: "wrap", Default: nil}})
	if p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'to_yaml'"))
	}
	// wrap is unsupported in golang, try to implement it later on
	o, err := b64.StdEncoding.DecodeString(in.String())
	u.CheckErr(err, "b64.StdEncoding.DecodeString of "+in.String())
	return exec.AsValue(string(o))
}

func CustomEnvironment() *exec.Environment {
	e := gonja.DefaultEnvironment
	if !e.Filters.Exists("regex_replace") {
		e.Filters.Register("regex_replace", filterFuncRegexReplace)
	}
	if !e.Filters.Exists("regex_search") {
		e.Filters.Register("regex_search", filterFuncRegexSearch)
	}
	if !e.Filters.Exists("to_yaml") {
		e.Filters.Register("to_yaml", filterFuncToYaml)
	}
	if !e.Filters.Exists("to_json") {
		e.Filters.Register("to_json", filterFuncToJson)
	}
	if !e.Filters.Exists("b64encode") {
		e.Filters.Register("b64encode", filterFuncB64Encode)
	}
	if !e.Filters.Exists("b64decode") {
		e.Filters.Register("b64decode", filterFuncB64Decode)
	}
	return e
}

func inspectTemplateFile(inputFilePath string) (needProcess bool, tempfilePath string, customConfig *config.Config) {
	file, err := os.Open(inputFilePath)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}
	defer file.Close()

	returnConfig := config.Config{
		BlockStartString:    "{%",
		BlockEndString:      "%}",
		VariableStartString: "{{",
		VariableEndString:   "}}",
		CommentStartString:  "{#",
		CommentEndString:    "#}",
		AutoEscape:          false,
		StrictUndefined:     false,
		TrimBlocks:          true,
		LeftStripBlocks:     true,
	}

	reader := bufio.NewReader(file)

	firstLine, err := reader.ReadString('\n')
	if err != nil {
		fmt.Println("[WARN] reading first line. Possibly File has only one line", err)
		return false, "", &returnConfig
	}
	// #jinja2:variable_start_string:'{$', variable_end_string:'$}', trim_blocks:True, lstrip_blocks:True
	if !strings.HasPrefix(firstLine, "#jinja2:") {
		return false, "", &returnConfig
	}

	for _, _token := range strings.Split(strings.TrimPrefix(firstLine, "#jinja2:"), ",") {
		_token0 := strings.TrimSpace(_token)
		_data := strings.Split(_token0, ":")
		switch _data[0] {
		case "variable_start_string":
			returnConfig.VariableStartString = strings.Trim(strings.Trim(_data[1], `'`), `"`)
		case "variable_end_string":
			returnConfig.VariableEndString = strings.Trim(strings.Trim(_data[1], `'`), `"`)
		case "trim_blocks":
			returnConfig.TrimBlocks = _data[1] == "True"
		case "lstrip_blocks":
			returnConfig.LeftStripBlocks = _data[1] == "True"
		}
	}
	// Create a temporary file for the rest of the content
	tempFile, err := os.CreateTemp("", "restContent_*.txt")
	if err != nil {
		fmt.Println("Error creating temporary file:", err)
		return
	}
	defer tempFile.Close()

	// Copy the rest of the content from the reader to the temporary file
	_, err = io.Copy(tempFile, reader)
	if err != nil {
		fmt.Println("Error copying the rest of the content to the temporary file:", err)
		return
	}

	return true, tempFile.Name(), &returnConfig
}

func templateFromBytesWithConfig(source []byte, config *config.Config) (*exec.Template, error) {
	rootID := fmt.Sprintf("root-%s", string(sha256.New().Sum(source)))

	loader, err := loaders.NewFileSystemLoader("")
	if err != nil {
		return nil, err
	}
	shiftedLoader, err := loaders.NewShiftedLoader(rootID, bytes.NewReader(source), loader)
	if err != nil {
		return nil, err
	}

	return exec.NewTemplate(rootID, config, shiftedLoader, CustomEnvironment())
}

func templateFromStringWithConfig(source string, config *config.Config) (*exec.Template, error) {
	return templateFromBytesWithConfig([]byte(source), config)
}

func templateFromFile(filepath string) (*exec.Template, error) {
	needToProcess, tempFile, parsedCfg := inspectTemplateFile(filepath)

	if !needToProcess {
		loader, err := loaders.NewFileSystemLoader(path.Dir(filepath))
		if err != nil {
			return nil, err
		}
		return exec.NewTemplate(path.Base(filepath), parsedCfg, loader, CustomEnvironment())
	}

	loader, err := loaders.NewFileSystemLoader(path.Dir(tempFile))
	if err != nil {
		return nil, err
	}
	return exec.NewTemplate(path.Base(tempFile), parsedCfg, loader, CustomEnvironment())
}

func TemplateFile(src, dest string, data map[string]interface{}, fileMode os.FileMode) {
	if fileMode == 0 {
		fileMode = 0755
	}
	// fmt.Printf("[INFO] Template %s => %s\n", src, dest)
	tmpl, err := templateFromFile(src)
	u.CheckErr(err, fmt.Sprintf("[ERROR] failed to create gonja context from src file %s\n", src))
	execContext := exec.NewContext(data)
	destFile, err := os.Create(dest)
	u.CheckErr(err, fmt.Sprintf("[ERROR] os.Create %s", dest))
	u.CheckErr(destFile.Chmod(fileMode), fmt.Sprintf("[ERROR] can not chmod %d for file %s\n", fileMode, dest))
	defer destFile.Close()
	u.CheckErr(err, fmt.Sprintf("[ERROR] Can not create destination file %s\n", dest))
	u.CheckErr(tmpl.Execute(destFile, execContext), "[ERROR] Can not template "+src+" => "+dest)
}

func TemplateString(srcString string, data map[string]interface{}) string {
	tmpl, err := templateFromStringWithConfig(srcString, &CustomConfig)
	u.CheckErr(err, fmt.Sprintf("[ERROR] failed to create gonja context from srcString %s\n", srcString))
	execContext := exec.NewContext(data)
	strBuff := strings.Builder{}
	u.CheckErr(tmpl.Execute(&strBuff, execContext), "[ERROR] Can not template")
	return strBuff.String()
}

func MapKeysToSlice(m map[string]int) []string {
	keys := make([]string, 0, len(m)) // Preallocate slice with the map's size
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}

func IncludeVars(filename string) map[string]interface{} {
	m := make(map[string]interface{})
	ValidateYamlFile(filename, &m)
	return m
}

func CreateDirTree(srcDirpath, targetRoot string) error {
	// Path should be absolute path. They should not overlap to avoid recursive loop
	if isExist, err := u.FileExists(srcDirpath); !isExist || err != nil {
		panic(fmt.Sprintf("[ERROR] src '%s' does not exist\n", srcDirpath))
	}
	os.Chdir(srcDirpath)
	filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			fmt.Printf("prevent panic by handling failure accessing a path %q: %v\n", srcDirpath, err)
			return err
		}

		if d.IsDir() {
			fmt.Printf("Going to create path %s\n", path)
			u.CheckErr(os.MkdirAll(filepath.Join(targetRoot, path), 0755), "ERROR MkdirAll")
		}

		return nil
	})
	return nil
}

func TemplateDirTree(srcDirpath, targetRoot string, tmplData map[string]interface{}) error {
	// Path should be absolute path. They should not overlap to avoid recursive loop
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
			TemplateFile(srcFile, destFile, tmplData, 0644)
		}

		return nil
	})
	return nil
}

// Custom JSON marshalling function
func CustomJsonMarshal(v interface{}) ([]byte, error) {
	converted := convertInterface(v)
	return json.Marshal(converted)
}
func CustomJsonMarshalIndent(v interface{}, indent int) ([]byte, error) {
	converted := convertInterface(v)
	return json.MarshalIndent(converted, "", strings.Repeat(" ", indent))
}
