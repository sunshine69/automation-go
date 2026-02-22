package gonja

import (
	"bytes"
	"crypto/sha256"
	b64 "encoding/base64"
	"fmt"
	"io/fs"

	json "github.com/json-iterator/go"

	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	u "github.com/sunshine69/golang-tools/utils"
	gonja "github.com/sunshine69/sonja/v2"
	"github.com/sunshine69/sonja/v2/config"
	"github.com/sunshine69/sonja/v2/exec"
	"github.com/sunshine69/sonja/v2/loaders"
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

// Monkey patching because arrays handling is broken
func GonjaCastToList(in *exec.Value) any {
	if in.IsList() {
		inCast := make([]interface{}, in.Len())
		for index := range inCast {
			item := exec.ToValue(in.Index(index).Val)
			inCast[index] = item.Val.Interface()
		}
		return exec.AsValue(inCast).ToGoSimpleType(true)
	}
	return nil
}

var filterFuncToJson exec.FilterFunction = func(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	// Done not mess around with trying to marshall error pipelines
	if in.IsError() {
		return in
	}

	p := params.Expect(0, []*exec.KwArg{{Name: "indent", Default: nil}})
	if p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'tojson'"))
	}

	casted := GonjaCastToList(in)
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
	o := u.Must(b64.StdEncoding.DecodeString(in.String()))
	return exec.AsValue(string(o))
}

var filterContainsAll exec.FilterFunction = func(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	// The sub_list passed as an argument: {{ main_list | contains_all(sub_list) }}

	if !in.IsList() || !params.Args[0].IsList() {
		return exec.AsValue(false)
	}

	mainList := GonjaCastToList(in).([]any)
	subList := GonjaCastToList(params.Args[0]).([]any)
	mainListStr := u.ConvertListIfaceToListStr(mainList)
	subListStr := u.ConvertListIfaceToListStr(subList)
	return exec.AsValue(u.SliceContainsItems(mainListStr, subListStr))
}

var filterContainsAny exec.FilterFunction = func(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	// The sub_list passed as an argument: {{ main_list | contains_all(sub_list) }}

	if !in.IsList() || !params.Args[0].IsList() {
		return exec.AsValue(false)
	}

	mainList := GonjaCastToList(in).([]any)
	subList := GonjaCastToList(params.Args[0]).([]any)
	mainListMap := u.SliceToMap(u.ConvertListIfaceToListStr(mainList))
	subListStr := u.ConvertListIfaceToListStr(subList)
	for _, item := range subListStr {
		if _, ok := mainListMap[item]; ok {
			return exec.AsValue(true)
		}
	}
	return exec.AsValue(false)
}

var filterKeys exec.FilterFunction = func(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	// The sub_list passed as an argument: {{ main_list | contains_all(sub_list) }}

	if !in.IsDict() {
		return exec.AsValue([]string{})
	}

	out := u.MapKeysToSlice(in.ToGoSimpleType(true).(map[any]any))
	return exec.AsValue(out)
}

func CustomEnvironment() *exec.Environment {
	// combine, extract, or dict2items ???
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
	if !e.Filters.Exists("contains") {
		e.Filters.Register("contains", filterContainsAll)
	}
	if !e.Filters.Exists("contains_any") {
		e.Filters.Register("contains_any", filterContainsAny)
	}
	if !e.Filters.Exists("keys") {
		e.Filters.Register("keys", filterKeys)
	}
	return e
}

// func inspectTemplateFile(inputFilePath string) (needProcess bool, tempfilePath string, customConfig *config.Config) {
// 	prefix := u.Getenv("JINJA2_CONFIG_LINE_PREFIX", `#jinja2:`)
// 	firstLine, newSrc, _, err := u.ReadFirstLineWithPrefix(inputFilePath, []string{prefix})
// 	if err != nil || newSrc == "" {
// 		return false, "", config.New()
// 	}
// 	foundConfig, config := parseJinja2Config(firstLine, prefix)
// 	if !foundConfig {
// 		newSrc = ""
// 		needProcess = false
// 	} else {
// 		needProcess = true
// 	}
// 	return needProcess, newSrc, config
// }

func inspectTemplateString(text string) (needProcess bool, remainText string, customConfig *config.Config) {
	firstLine, newSrc := u.SplitFirstLine(text)
	if newSrc == "" {
		return false, "", config.New()
	}
	prefix := u.Getenv("JINJA2_CONFIG_LINE_PREFIX", `#jinja2:`)
	foundConfig, config := parseJinja2Config(firstLine, prefix)
	if !foundConfig {
		newSrc = ""
		needProcess = false
	}
	return needProcess, newSrc, config
}

func inspectTemplateBytes(dataB []byte) (needProcess bool, remainB []byte, customConfig *config.Config) {
	firstLine, newSrc := u.SplitFirstLine(dataB)
	if bytes.Equal(newSrc, []byte{}) {
		return false, []byte{}, config.New()
	}
	prefix := u.Getenv("JINJA2_CONFIG_LINE_PREFIX", `#jinja2:`)
	foundConfig, config := parseJinja2Config(string(firstLine), prefix)
	if !foundConfig {
		newSrc = []byte{}
		needProcess = false
	}
	return needProcess, newSrc, config
}

func parseJinja2Config(firstLine, prefix string) (foundConfig bool, jinja2config *config.Config) {
	returnConfig, foundConfig := config.New(), false
	for _, _token := range strings.Split(strings.TrimPrefix(firstLine, prefix), ",") {
		_token0 := strings.TrimSpace(_token)
		_data := strings.Split(_token0, ":")
		switch _data[0] {
		case "variable_start_string":
			foundConfig = true
			returnConfig.VariableStartString = strings.Trim(strings.Trim(_data[1], `'`), `"`)
		case "variable_end_string":
			foundConfig = true
			returnConfig.VariableEndString = strings.Trim(strings.Trim(_data[1], `'`), `"`)
		case "trim_blocks":
			foundConfig = true
			returnConfig.TrimBlocks = _data[1] == "True"
		case "lstrip_blocks":
			foundConfig = true
			returnConfig.LeftStripBlocks = _data[1] == "True"
		}
	}
	return foundConfig, returnConfig
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

func TemplateFromStringWithConfig(source string, config *config.Config) (*exec.Template, error) {
	return templateFromBytesWithConfig([]byte(source), config)
}

func templateFromFileWithConfig(filepath string, config *config.Config) (*exec.Template, error) {
	loader, err := loaders.NewFileSystemLoader(path.Dir(filepath))
	t, err := exec.NewTemplate(path.Base(filepath), config, loader, CustomEnvironment())
	return t, err
}

// Template a file using template string and convert windows new line to unix. This is
// work around the gonja2 windows new line problem
func TemplateFile(src, dest string, data map[string]interface{}, fileMode os.FileMode) {
	if fileMode == 0 {
		fileMode = 0o777
	}
	if contentb, err := os.ReadFile(src); err == nil {
		outb := u.Must(TemplateBytes(contentb, data))
		u.CheckErr(os.WriteFile(dest, outb, fileMode), "TemplateFile")
	}
}

func parseConfigVarArgs(opt []string) (*config.Config, map[string]string) {
	cfg := config.New()
	cfg.TrimBlocks = true
	cfg.LeftStripBlocks = true

	extraConfig := map[string]string{"replace_new_line": "True"}

	optLength := len(opt)
	if optLength >= 2 {
		for i := 0; i <= optLength-2; i += 2 {
			switch opt[i] {
			case "variable_start_string":
				cfg.VariableStartString = opt[i+1]
			case "variable_end_string":
				cfg.VariableEndString = opt[i+1]
			case "trim_blocks":
				cfg.TrimBlocks = opt[i+1] == "True"
			case "lstrip_blocks":
				cfg.LeftStripBlocks = opt[i+1] == "True"
			case "auto_escape":
				cfg.AutoEscape = opt[i+1] == "True"
			case "comment_start_string":
				cfg.CommentStartString = opt[i+1]
			case "comment_end_string":
				cfg.CommentEndString = opt[i+1]
			case "strict_undefined":
				cfg.StrictUndefined = opt[i+1] == "True"
			default:
				extraConfig[opt[i]] = opt[i+1]
			}
		}
	}
	return cfg, extraConfig
}

// Still not pass tests if run on windows - Dont use. Run a file with \r\n on win 8 caused error in the engine. Run on unix does
// not return error but these lines are not removed at all. Might be the latest version works?
// This uses the template file for real, not reading all data and do the replace \r\n => \n
// This func is suiatable to run on server as it wont crash but return err if tehre is err and have the most comprehensive options
//
// It also take the config options and do not parse the config line in the file so there is no intermedeate copy to temp file
// it a bit tiny faster esp when template large files > 10Mb for example
// Note there is known the windows new line problems if you this func. To replace_new_line set it to true
func TemplateFileWithConfig(src, dest string, data map[string]interface{}, fileMode os.FileMode, opt ...string) error {
	if fileMode == 0 {
		fileMode = 0o777
	}

	cfg, extraConfig := parseConfigVarArgs(opt)
	// By default teh replace new line is set to True from parseConfigVarArgs
	if extraConfig["replace_new_line"] == "True" {
		if dataB, err := os.ReadFile(src); err == nil {
			outB, err := TemplateBytes(dataB, data)
			if err != nil {
				return err
			}
			return os.WriteFile(dest, outB, fileMode)
		} else {
			return err
		}
	}
	// Now we dont replace new line - we use gonja filesystem loader directly from now on
	if extraConfig["DEBUG"] == "True" {
		fmt.Fprintf(os.Stderr, "[DEBUG] %s\n", u.JsonDump(cfg, ""))
	}

	tmpl, err := templateFromFileWithConfig(src, cfg)
	if err != nil {
		return err
	}
	execContext := exec.NewContext(data)
	destFile, err := os.Create(dest)
	if err != nil {
		return err
	}
	if err := u.CheckErrNonFatal(destFile.Chmod(fileMode), fmt.Sprintf("[ERROR] can not chmod %d for file %s\n", fileMode, dest)); err != nil {
		return err
	}
	defer destFile.Close()
	if err := u.CheckErrNonFatal(tmpl.Execute(destFile, execContext), "[ERROR] Can not template "+src+" => "+dest); err != nil {
		return err
	}
	return nil
}

func TemplateStringWithConfig(srcString string, data map[string]interface{}, opt ...string) (string, error) {
	cfg, extraCfg := parseConfigVarArgs(opt)
	if extraCfg["replace_new_line"] == "True" {
		srcString = strings.ReplaceAll(srcString, "\r\n", "\n")
	}
	tmpl, err := TemplateFromStringWithConfig(srcString, cfg)
	if err != nil {
		return "", err
	}
	execContext := exec.NewContext(data)
	if extraCfg["DEBUG"] == "True" {
		fmt.Fprintf(os.Stderr, "CFG: %s\n", u.JsonDump(cfg, ""))
	}
	return tmpl.ExecuteToString(execContext)
}

func TemplateString(srcString string, data map[string]interface{}) string {
	_, newSrc, CustomConfig := inspectTemplateString(srcString)
	if newSrc == "" {
		newSrc = srcString
	}
	tmpl := u.Must(TemplateFromStringWithConfig(newSrc, CustomConfig))
	execContext := exec.NewContext(data)
	return u.Must(tmpl.ExecuteToString(execContext))
}

func TemplateBytes(srcB []byte, data map[string]any) ([]byte, error) {
	_, newSrc, CustomConfig := inspectTemplateBytes(srcB)
	if bytes.Equal(newSrc, []byte{}) {
		newSrc = srcB
	}

	srcB = bytes.ReplaceAll(newSrc, []byte("\r\n"), []byte("\n"))

	tmpl, err := templateFromBytesWithConfig(newSrc, CustomConfig)
	if err != nil {
		return nil, err
	}
	return tmpl.ExecuteToBytes(exec.NewContext(data))
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
