package lib

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"unicode"

	json "github.com/json-iterator/go"
	u "github.com/sunshine69/golang-tools/utils"
	"github.com/tidwall/gjson"
	"gopkg.in/ini.v1"
	"gopkg.in/yaml.v3"
)

type StructInfo struct {
	Name       string
	FieldName  []string
	FieldType  map[string]string
	FieldValue map[string]any
	TagCapture map[string][][]string
}

// Give it a struct and a tag pattern to capture the tag content - return a map of string which is the struct Field name, point to a map of
// string which is the capture in the pattern
func ReflectStruct(astruct any, tagPtn string) StructInfo {
	if tagPtn == "" {
		tagPtn = `db:"([^"]+)"`
	}
	o := StructInfo{}
	tagExtractPtn := regexp.MustCompile(tagPtn)

	rf := reflect.TypeOf(astruct)
	o.Name = rf.Name()
	if rf.Kind().String() != "struct" {
		panic("I need a struct")
	}
	rValue := reflect.ValueOf(astruct)
	o.FieldName = []string{}
	o.FieldType = map[string]string{}
	o.FieldValue = map[string]any{}
	o.TagCapture = map[string][][]string{}
	for i := 0; i < rf.NumField(); i++ {
		f := rf.Field(i)
		o.FieldName = append(o.FieldName, f.Name)
		fieldValue := rValue.Field(i)
		o.FieldType[f.Name] = fieldValue.Type().String()
		o.TagCapture[f.Name] = [][]string{}
		switch fieldValue.Type().String() {
		case "string":
			o.FieldValue[f.Name] = fieldValue.String()
		case "int64":
			o.FieldValue[f.Name] = fieldValue.Int()
		default:
			fmt.Printf("Unsupported field type " + fieldValue.Type().String())
		}
		if ext := tagExtractPtn.FindAllStringSubmatch(string(f.Tag), -1); ext != nil {
			o.TagCapture[f.Name] = append(o.TagCapture[f.Name], ext...)
		}
	}
	return o
}

// Take a slice and a function return new slice with the value is the result of the function called for each item
// Similar to list walk in python
func SliceMap[T, V any](ts []T, fn func(T) *V) []V {
	result := []V{}
	for _, t := range ts {
		_v := fn(t)
		if _v != nil {
			result = append(result, *_v)
		}
	}
	return result
}

// Similar to the python dict.keys()
func MapKeysToSlice(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m)) // Preallocate slice with the map's size
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}

// Validate a yaml file and load it into a map
func IncludeVars(filename string) map[string]interface{} {
	m := make(map[string]interface{})
	ValidateYamlFile(filename, &m)
	return m
}

// Read the first line of a file
func ReadFirstLineOfFile(filepath string) string {
	file, err := os.Open(filepath)
	u.CheckErr(err, "[ERROR] readFirstLineOfFile")
	defer file.Close()

	scanner := bufio.NewScanner(file)

	if scanner.Scan() {
		// Check for errors during scanning
		u.CheckErr(scanner.Err(), "[ERROR] readFirstLineOfFile error scanning file")
		return scanner.Text()
	} else {
		// Handle the case where there's no line in the file
		return ""
	}
}

// Function to convert interface{} => list string
func ConvertListIfaceToListStr(in interface{}) []string {
	o := []string{}
	for _, v := range in.([]interface{}) {
		o = append(o, v.(string))
	}
	return o
}

func InterfaceToStringList(in []interface{}) []string {
	o := []string{}
	for _, v := range in {
		o = append(o, v.(string))
	}
	return o
}

func InterfaceToStringMap(in map[string]interface{}) map[string]string {
	o := map[string]string{}
	for k, v := range in {
		o[k] = v.(string)
	}
	return o
}

// SliceToMap convert a slice of any comparable into a map which can set the value later on
func SliceToMap[T comparable](slice []T) map[T]interface{} {
	set := make(map[T]interface{})
	for _, element := range slice {
		set[element] = nil
	}
	return set
}

func AssertInt64ValueForMap(input map[string]interface{}) map[string]interface{} {
	for k, v := range input {
		if v, ok := v.(float64); ok {
			input[k] = int64(v)
		}
	}
	return input
}

// JsonToMap take a json string and decode it into a map[string]interface{}
func JsonToMap(jsonStr string) map[string]interface{} {
	result := make(map[string]interface{})
	json.Unmarshal([]byte(jsonStr), &result)
	return AssertInt64ValueForMap(result)
}

// Take a struct and convert into a map[string]any - the key of the map is the struct field name, and the value is the struct field value.
// This is useful to pass it to the gop template to render the struct value
func ConvertStruct2Map[T any](t T) ([]string, map[string]any) {
	sInfo := ReflectStruct(t, "")
	out := map[string]any{}
	for _, f := range sInfo.FieldName {
		out[f] = sInfo.FieldValue[f]
	}
	return sInfo.FieldName, out
}

func ParseJsonReqBodyToMap(r *http.Request) map[string]interface{} {
	switch r.Method {
	case "POST", "PUT", "DELETE":
		jsonBytes := bytes.Buffer{}
		if _, err := io.Copy(&jsonBytes, r.Body); err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] ParseJSONToMap loading request body - %s\n", err.Error())
			return nil
		}
		defer r.Body.Close()
		return JsonToMap(string(jsonBytes.Bytes()))
	default:
		fmt.Fprintf(os.Stderr, "[ERROR] ParseJSONToMap Do not call me with this method - %s\n", r.Method)
		return nil
	}
}

// ParseJSON parses the raw JSON body from an HTTP request into the specified struct.
func ParseJsonReqBodyToStruct[T any](r *http.Request) *T {
	switch r.Method {
	case "POST", "PUT", "DELETE":
		decoder := json.NewDecoder(r.Body)
		defer r.Body.Close()
		var data T
		if err := decoder.Decode(&data); err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] parsing JSON - %s\n", err.Error())
			return nil
		}
		return &data
	default:
		fmt.Fprintf(os.Stderr, "[ERROR] ParseJSON Do not call me with this method - %s\n", r.Method)
		return nil
	}
}

func ItemExists[T comparable](item T, set map[T]interface{}) bool {
	_, exists := set[item]
	return exists
}

func IniGetVal(inifilepath, section, option string) string {
	cfg, err := ini.Load(inifilepath)
	if err != nil {
		fmt.Println("Error loading INI file:", err)
		os.Exit(1)
	}
	// Get an option value from a section
	return cfg.Section(section).Key(option).String()
}

func IniSetVal(inifilepath, section, option, value string) {
	cfg, err := ini.Load(inifilepath)
	if err != nil {
		fmt.Println("Error loading INI file:", err)
		os.Exit(1)
	}
	// Get an option value from a section
	cfg.Section(section).Key(option).SetValue(value)
	cfg.SaveToIndent(inifilepath, "  ")
}

// Given a key as string, may have dot like objecta.field_a.b. and a map[string]interface{}
// check if the map has the key path point to a non nil value; return true if value exists otherwise
func validateAKeyWithDotInAmap(key string, vars map[string]interface{}) bool {
	jsonB := u.JsonDumpByte(vars, "")
	if !gjson.ValidBytes(jsonB) {
		panic("Invalid jsonstring input")
	}
	r := gjson.GetBytes(jsonB, key)
	return r.Exists()
}

// Validate helm template. Pretty simple for now, not assess the set new var directive or include
// directive or long access var within range etc.
// `trivy` and `helm lint` with k8s validation should cover that job
// This only deals with when var is not defined, helm content rendered as empty string.
// `helm lint` wont give you error for that.
// Walk through the template, search for all string pattern with {{ .Values.<XXX> }} -
// then extract the var name.
// Load the helm values files into map, merge them and check the var name (or path access) in there.
// If not print outout error
// If there is helm template `if` statement to test the value then do not fail
// If there is a helm `default` function of filter to test the value and set the default value then do not fail
func HelmChartValidation(chartPath string, valuesFile []string) bool {
	vars := map[string]interface{}{}
	for _, fn := range valuesFile {
		ValidateYamlFile(fn, &vars)
	}

	valuesPtn := regexp.MustCompile(`\{\{[\-]{0,1}[\s]*\.Values\.([^\s\}]+)[\s]+[\-]{0,1}\}\}`)
	valuesInIfStatementPtn := regexp.MustCompile(`\{\{[\-]{0,1}[\s]*if[\s]+\.Values\.([^\s\}]+)[\s]+[\-]{0,1}\}\}`)
	valuesInDefaultFuncPtn := regexp.MustCompile(`\{\{[\-]{0,1}[\s]*default[\s]+[^\s]+\.Values\.([^\s\}]+)[\s]+[\-]{0,1}\}\}`)
	valuesInDefaultFilterPtn := regexp.MustCompile(`\{\{[\-]{0,1}[\s]+\.Values\.([^\s\}]+)[\s]+\|[\s]+default[\s]+[^\s]+[\s]+[\-]{0,1}\}\}`)

	helmTemplateFileVarList := map[string][]string{}
	errorLogsLine := []string{}

	err := filepath.Walk(fmt.Sprintf("%s/templates", chartPath), func(path string, info fs.FileInfo, err1 error) error {
		if err1 != nil {
			return err1
		}
		if info.IsDir() {
			return nil
		}
		fcontentb, err := os.ReadFile(path)
		u.CheckErr(err, "HelmChartValidation ReadFile")

		findResIf := valuesInIfStatementPtn.FindAllSubmatch(fcontentb, -1)
		tempListExcludeMap := map[string]struct{}{}
		for _, res := range findResIf {
			tempListExcludeMap[string(res[1])] = struct{}{}
		}

		findResDefaultFunc := valuesInDefaultFuncPtn.FindAllSubmatch(fcontentb, -1)
		for _, res := range findResDefaultFunc {
			tempListExcludeMap[string(res[1])] = struct{}{}
		}

		findResDefaultFilter := valuesInDefaultFilterPtn.FindAllSubmatch(fcontentb, -1)
		for _, res := range findResDefaultFilter {
			tempListExcludeMap[string(res[1])] = struct{}{}
		}

		findRes := valuesPtn.FindAllSubmatch(fcontentb, -1)
		tempList := []string{}
		for _, res := range findRes {
			_v := string(res[1])
			if _, ok := tempListExcludeMap[_v]; !ok {
				tempList = append(tempList, _v)
			}
		}

		helmTemplateFileVarList[info.Name()] = tempList
		return nil
	})
	u.CheckErr(err, "filepath.Walk")

	for k, v := range helmTemplateFileVarList {
		for _, varname := range v {
			if !validateAKeyWithDotInAmap(varname, vars) {
				errorLogsLine = append(errorLogsLine, fmt.Sprintf("Var '%s' in template file %s is not defined in values file\n", varname, k))
			}
		}
	}
	if len(errorLogsLine) > 0 {
		errMsg := strings.Join(errorLogsLine, "\n")
		panic(errMsg)
	}
	return true
}

// MaskCredential RegexPattern
var MaskCredentialPattern *regexp.Regexp = regexp.MustCompile(`(?i)(password|token|pass|passkey|secret|secret_key|access_key|PAT)([:=]{1,1})[\s]*[^\s]+`)

// Mask all credentials pattern
func MaskCredential(inputstr string) string {
	return MaskCredentialPattern.ReplaceAllString(inputstr, "$1$2 *****")
}

// Mask all credentials pattern
func MaskCredentialByte(inputbytes []byte) string {
	return string(MaskCredentialPattern.ReplaceAll(inputbytes, []byte("$1$2 *****")))
}

// Validate yaml files. Optionally return the unmarshalled object if you pass yamlobj not nil
func ValidateYamlFile(yaml_file string, yamlobj *map[string]interface{}) map[string]interface{} {
	data, err := os.ReadFile(yaml_file)
	u.CheckErr(err, "ValidateYamlFile ReadFile")
	if yamlobj == nil {
		t := map[string]interface{}{}
		yamlobj = &t
	}
	err = yaml.Unmarshal(data, &yamlobj)
	if err1 := u.CheckErrNonFatal(err, "ValidateYamlFile Unmarshal"); err1 != nil {
		fmt.Printf("Yaml content has error:\n%s\n", MaskCredentialByte(data))
		panic(err1.Error())
	}
	return *yamlobj
}

// Validate directory containing yaml files. Optionally return the unmarshalled object if you pass yamlobj not nil
func ValidateYamlDir(yaml_dir string, yamlobj *map[string]interface{}) bool {
	if yamlobj == nil {
		t := map[string]interface{}{}
		yamlobj = &t
	}
	filepath.Walk(yaml_dir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		ext := filepath.Ext(info.Name())
		if ext == ".yaml" || ext == ".yml" {
			ValidateYamlFile(path, yamlobj)
		}
		return nil
	})
	return true
}

// ReplaceAllFuncN extends regexp.Regexp to support count of replacements for []byte
func ReplaceAllFuncN(re *regexp.Regexp, src []byte, repl func([]int, [][]byte) []byte, n int) ([]byte, int) {
	if n == 0 {
		return src, 0
	}

	matches := re.FindAllSubmatchIndex(src, n)
	if matches == nil {
		return src, 0
	}

	var result bytes.Buffer
	lastIndex := 0
	replacementCount := 0
	for _, match := range matches {
		result.Write(src[lastIndex:match[0]])
		submatches := make([][]byte, (len(match) / 2))
		for i := 0; i < len(match); i += 2 {
			if match[i] >= 0 && match[i+1] >= 0 {
				submatches[i/2] = src[match[i]:match[i+1]]
			} else {
				submatches[i/2] = nil
			}
		}
		result.Write(repl(match, submatches))
		lastIndex = match[1]
		replacementCount++
	}
	result.Write(src[lastIndex:])

	return result.Bytes(), replacementCount
}

func ReplacePattern(input []byte, pattern string, repl string, count int) ([]byte, int) {
	re := regexp.MustCompile(pattern)
	replaceFunc := func(matchIndex []int, submatches [][]byte) []byte {
		expandedRepl := []byte(repl)
		for i, submatch := range submatches {
			if submatch != nil {
				placeholder := fmt.Sprintf("$%d", i)
				expandedRepl = bytes.Replace(expandedRepl, []byte(placeholder), submatch, -1)
			}
		}
		return expandedRepl
	}

	return ReplaceAllFuncN(re, input, replaceFunc, count)
}

// Do regex search and replace in a file
func SearchReplaceFile(filename, ptn, repl string, count int, backup bool) int {
	finfo, err := os.Stat(filename)
	u.CheckErr(err, "SearchReplaceFile Stat")
	fmode := finfo.Mode()
	if !(fmode.IsRegular()) {
		panic("CopyFile: non-regular destination file")
	}
	data, err := os.ReadFile(filename)
	u.CheckErr(err, "SearchReplaceFile ReadFile")
	if backup {
		os.WriteFile(filename+".bak", data, fmode)
	}
	dataout, count := ReplacePattern(data, ptn, repl, count)
	os.WriteFile(filename, dataout, fmode)
	return count
}

func SearchReplaceString(instring, ptn, repl string, count int) string {
	o, _ := ReplacePattern([]byte(instring), ptn, repl, count)
	return string(o)
}

type LineInfileOpt struct {
	Insertafter   string
	Insertbefore  string
	Line          string
	LineNo        int
	Path          string
	Regexp        string
	Search_string string
	State         string
	Backup        bool
	ReplaceAll    bool
}

func NewLineInfileOpt(opt *LineInfileOpt) *LineInfileOpt {
	if opt.State == "" {
		opt.State = "present"
	}
	return opt
}

// Simulate ansible lineinfile module. There are some difference intentionaly to avoid confusing behaviour and reduce complexbility
// No option backref, the default behaviour is yes. That is when Regex is set it never add new line. To add new line use search_string or insert_after, insert_before opts.
// TODO bugs still when state=absent :P
func LineInFile(filename string, opt *LineInfileOpt) (err error, changed bool) {
	var returnFunc = func(err error, changed bool) (error, bool) {
		if !changed || !opt.Backup {
			os.Remove(filename + ".bak")
		}
		return err, changed
	}
	if opt.State == "" {
		opt.State = "present"
	}
	finfo, err := os.Stat(filename)
	if err1 := u.CheckErrNonFatal(err, "LineInFile Stat"); err1 != nil {
		return err1, false
	}
	fmode := finfo.Mode()
	if !(fmode.IsRegular()) {
		return fmt.Errorf("LineInFile: non-regular destination file %s", filename), false
	}
	if opt.Search_string != "" && opt.Regexp != "" {
		panic("[ERROR] conflicting option. Search_string and Regexp can not be both set")
	}
	if opt.Insertafter != "" && opt.Insertbefore != "" {
		panic("[ERROR] conflicting option. Insertafter and Insertbefore can not be both set")
	}
	if opt.LineNo > 0 && opt.Regexp != "" {
		panic("[ERROR] conflicting option. LineNo and Regexp can not be both set")
	}
	data, err := os.ReadFile(filename)
	if err1 := u.CheckErrNonFatal(err, "LineInFile ReadFile"); err1 != nil {
		return err1, false
	}

	if opt.Backup && opt.State != "print" {
		os.WriteFile(filename+".bak", data, fmode)
	}
	changed = false
	optLineB := []byte(opt.Line)
	datalines := bytes.Split(data, []byte("\n"))
	// ansible lineinfile is confusing. If set search_string and insertafter or inserbefore if search found the line is replaced and the other options has no effect. Unless search_string is not found then they will do it. Why we need that?
	// Basically the priority is search_string == regexp (thus they are mutually exclusive); and then insertafter or before. They can be all regex except search_string
	// If state is absent it remove all line matching the string, ignore the `line` param
	processAbsentLines := func(line_exist_idx map[int]interface{}, index_list []int, search_string_found bool) (error, bool) {
		d, d2 := []string{}, map[int]string{}
		// fmt.Printf("DEBUG line_exist_idx %v index_list %v search_string_found %v\n", line_exist_idx, index_list, search_string_found)
		if len(line_exist_idx) == 0 && len(index_list) == 0 {
			return nil, false
		}
		for idx, l := range datalines {
			_l := string(l)
			d = append(d, _l)
			// line_exist_idx output of the case of matched the whole line
			if _, ok := line_exist_idx[idx]; ok {
				d2[idx] = _l
			}
		}
		// index_list is the outcome of the search_string/regex opt (search raw string).
		for _, idx := range index_list {
			if search_string_found {
				d2[idx] = d[idx] // remember the value to this map
			}
		}
		// fmt.Printf("DEBUG d2 %s\n", u.JsonDump(d2, "  "))
		if opt.State == "print" {
			o := map[string]interface{}{
				"file":          filename,
				"matched_lines": d2,
			}
			fmt.Printf("%s\n", u.JsonDump(o, "  "))
		} else {
			for _, v := range d2 { // then remove by val here.
				d = u.RemoveItemByVal(d, v)
			}
			os.WriteFile(filename, []byte(strings.Join(d, "\n")), fmode)
		}
		return nil, true
	}
	// Now we process case by case
	if opt.Search_string != "" || opt.LineNo > 0 { // Match the whole line or we have line number. This is derterministic behaviour
		search_string_found, line_exist_idx := true, map[int]interface{}{}
		index_list := []int{}
		if opt.LineNo > 0 { // If we have line number we ignore the search string to be fast
			index_list = append(index_list, opt.LineNo-1)
		} else {
			for idx, lineb := range datalines {
				if bytes.Contains(lineb, []byte(opt.Search_string)) {
					index_list = append(index_list, idx)
				}
				if bytes.Equal(lineb, optLineB) { // Line already exists
					if opt.State == "present" {
						return returnFunc(nil, changed)
					} else {
						if !bytes.Equal(optLineB, []byte("")) {
							line_exist_idx[idx] = nil
						}
					}
				}
			}
		}
		if len(index_list) == 0 { // Did not find any search string. Will look insertafter  and before
			search_string_found = false
			ptnstring := opt.Insertafter
			if ptnstring == "" {
				ptnstring = opt.Insertbefore
			}
			if ptnstring != "" {
				ptn := regexp.MustCompile(ptnstring)
				for idx, lineb := range datalines {
					if ptn.Match(lineb) {
						index_list = append(index_list, idx)
					}
				}
			}
		}
		if len(index_list) == 0 && len(line_exist_idx) == 0 {
			// Can not find any insert_XXX match. Just add a new line at the end by setting this to the last
			index_list = append(index_list, len(datalines)-1)
		}
		switch opt.State {
		case "absent":
			return returnFunc(processAbsentLines(line_exist_idx, index_list, search_string_found))
		case "present":
			last := index_list[len(index_list)-1]
			if search_string_found {
				if !opt.ReplaceAll {
					datalines[last] = optLineB
				} else {
					for _, idx := range index_list {
						datalines[idx] = optLineB
					}
				}
			} else {
				if opt.Insertafter != "" {
					datalines = InsertItemAfter(datalines, last, optLineB)
				} else if opt.Insertbefore != "" {
					datalines = InsertItemBefore(datalines, last, optLineB)
				} else { // to the end as always
					datalines = InsertItemAfter(datalines, last, optLineB)
				}
			}
			os.WriteFile(filename, []byte(bytes.Join(datalines, []byte("\n"))), fmode)
			changed = true
		case "print":
			return returnFunc(processAbsentLines(line_exist_idx, index_list, search_string_found))
		}
	}
	// Assume the behaviour is the same as search_string for Regex, just it is a regex now. So if it matches then the line matched will be replaced. If no match then process the insertbefore or after
	if opt.Regexp != "" {
		search_string_found := true
		regex_ptn := regexp.MustCompile(opt.Regexp)
		index_list := []int{}
		matchesMap := map[int][][]byte{}
		line_exist_idx := map[int]interface{}{}

		for idx, lineb := range datalines {
			matches := regex_ptn.FindSubmatch(lineb)
			if len(matches) > 0 || matches != nil {
				index_list = append(index_list, idx)
				matchesMap[idx] = matches
			}
		}
		if len(index_list) == 0 { // Did not find any search string. Will look insertafter  and before
			search_string_found = false
			for idx, lineb := range datalines {
				if bytes.Equal(lineb, optLineB) { // Line already exists
					if opt.State == "present" {
						return returnFunc(nil, changed)
					} else {
						if !bytes.Equal(optLineB, []byte("")) {
							line_exist_idx[idx] = nil
						}
					}
				}
			}
			ptnstring := opt.Insertafter
			if ptnstring == "" {
				ptnstring = opt.Insertbefore
			}
			if ptnstring == "" {
				return returnFunc(nil, false)
			}
			ptn := regexp.MustCompile(ptnstring)
			for idx, lineb := range datalines {
				if ptn.Match(lineb) {
					index_list = append(index_list, idx)
				}
			}
		}
		if len(index_list) == 0 && len(line_exist_idx) == 0 {
			// Can not find any insert_XXX match. Just add a new line at the end by setting this to the last
			index_list = append(index_list, len(datalines)-1)
		}
		switch opt.State {
		case "absent":
			return returnFunc(processAbsentLines(line_exist_idx, index_list, search_string_found))
		case "present":
			last := index_list[len(index_list)-1]
			if search_string_found {
				// Expanding submatch
				if !opt.ReplaceAll {
					for i, submatch := range matchesMap[last] {
						if submatch != nil {
							placeholder := fmt.Sprintf("$%d", i)
							optLineB = bytes.Replace(optLineB, []byte(placeholder), submatch, -1)
						}
					}
					datalines[last] = optLineB
				} else {
					for _, line := range index_list {
						for i, submatch := range matchesMap[line] {
							if submatch != nil {
								placeholder := fmt.Sprintf("$%d", i)
								optLineB = bytes.Replace(optLineB, []byte(placeholder), submatch, -1)
								datalines[line] = optLineB
							}
						}
					}
				}
			} else {
				if opt.Insertafter != "" {
					datalines = InsertItemAfter(datalines, last, optLineB)
				} else if opt.Insertbefore != "" {
					datalines = InsertItemBefore(datalines, last, optLineB)
				} else { // Insert to the last then :P
					datalines = InsertItemAfter(datalines, last, optLineB)
				}
			}
			os.WriteFile(filename, []byte(bytes.Join(datalines, []byte("\n"))), fmode)
			changed = true
		case "print":
			return returnFunc(processAbsentLines(line_exist_idx, index_list, search_string_found))
		}
	}
	return err, changed
}

// InsertItemBefore inserts an item into a slice before a specified index
func InsertItemBefore[T any](slice []T, index int, item T) []T {
	if index < 0 || index > len(slice) {
		panic("InsertItemBefore: index out of range")
	}
	slice = append(slice[:index], append([]T{item}, slice[index:]...)...)
	return slice
}

// InsertItemAfter inserts an item into a slice after a specified index
func InsertItemAfter[T any](slice []T, index int, item T) []T {
	if index < -1 || index >= len(slice) {
		panic("InsertItemAfter: index out of range")
	}
	slice = append(slice[:index+1], append([]T{item}, slice[index+1:]...)...)
	return slice
}

func IsBinaryFile(filePath string) (bool, error) {
	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		return false, err
	}
	defer file.Close()

	// Read the first 512 bytes
	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil {
		return false, err
	}

	// Detect the content type
	contentType := http.DetectContentType(buffer[:n])

	// Check if the content type indicates a binary file
	switch contentType {
	case "application/octet-stream", "application/x-executable", "application/x-mach-binary":
		return true, nil
	default:
		if len(contentType) > 0 && contentType[:5] == "text/" {
			return false, nil
		}
		return true, nil
	}
}

func IsBinaryFileSimple(filePath string) (bool, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return false, err
	}
	defer file.Close()

	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil {
		return false, err
	}

	for i := 0; i < n; i++ {
		if buffer[i] == 0 {
			return true, nil
		}
		if buffer[i] < 0x20 && buffer[i] != 0x09 && buffer[i] != 0x0A && buffer[i] != 0x0D {
			return true, nil
		}
	}
	return false, nil
}

// Heuristic detect if the values is likely a real password etc
// possible values for check_mode: letter, digit, letter+digit, letter+digit+word
// if any other values it will be the same effect as letter+digit+word+special
// if you provide `letter` means the function will detect letter(s) in the value AND as long as it is greater than the
// entropy_threshold level it will return true
// Same `letter+digit` - the value must contain at least letter and digit so on
// word means if the value is an english word it return false (not 100% if entropy is high it might return true)
// The word check requires `words_file_path` to be set to a path of the words file; if the value is empty string then it
// have the default value is "words.txt". You need to be sure to create the file yourself.
// Link to download https://github.com/dwyl/english-words/blob/master/words.txt
// These rules to reduce the false positive detection as people might put there as an example of password rather then real password,
// we only want to spot out real password.
func IsLikelyPasswordOrToken(value, check_mode, words_file_path string, word_len int, entropy_threshold float64) bool {
	// Check length
	if len(value) < 6 || len(value) > 64 {
		// fmt.Printf("[WARN] Skipping %s as len is not > 8 and < 64\n", value)
		return false
	}
	if words_file_path == "" {
		words_file_path = "words.txt"
	}
	if word_len == 0 {
		word_len = 4
	}
	// Check for character variety
	var hasDigit, hasSpecial, hasUpper, hasLower bool
	for _, char := range value {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsDigit(char):
			hasDigit = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}
	if entropy_threshold == 0 {
		entropy_threshold = 2.5
	}
	if entropy := calculateEntropy(value); entropy <= entropy_threshold {
		return false
	}

	var word_dict *map[string]struct{} = nil
	hasWord := false
	detectHasWord := func(word_dict *map[string]struct{}) (bool, *map[string]struct{}) {
		if word_dict == nil {
			_word_dict, err := loadDictionary(words_file_path, word_len)
			// cache word_dict
			word_dict = &_word_dict
			u.CheckErr(err, "IsLikelyPasswordOrToken loadDictionary")
		}
		if ContainsDictionaryWord(value, *word_dict) {
			return true, word_dict
		}
		return false, word_dict
	}

	switch check_mode {
	case "letter":
		return hasUpper && hasLower
	case "digit":
		return hasDigit
	case "special":
		return hasSpecial
	case "letter+digit":
		return hasUpper && hasLower && hasDigit
	case "letter+word":
		hasWord, _ = detectHasWord(word_dict)
		if hasWord {
			return false
		}
		return hasUpper && hasLower
	case "letter+digit+word":
		hasWord, _ = detectHasWord(word_dict)
		if hasWord {
			return false
		}
		return hasUpper && hasLower && hasDigit
	default:
		hasWord, _ = detectHasWord(word_dict)
		if hasWord {
			return false
		}
		return hasUpper && hasLower && hasDigit && hasSpecial
	}

}

func calculateEntropy(s string) float64 {
	// Count the frequency of each character
	frequency := make(map[rune]int)
	for _, char := range s {
		frequency[char]++
	}

	// Calculate the entropy
	var entropy float64
	length := float64(len(s))
	for _, count := range frequency {
		p := float64(count) / length
		entropy -= p * math.Log2(p)
	}

	return entropy
}

// Load dictionary words from a file and return a map for faster lookups
func loadDictionary(filename string, word_len int) (map[string]struct{}, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	if word_len == 0 || word_len == -1 {
		word_len = 4
	}
	dictionary := make(map[string]struct{})
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		word := strings.ToLower(scanner.Text())
		if len(word) >= word_len {
			dictionary[word] = struct{}{}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return dictionary, nil
}

// Function to check if a string contains any dictionary words using a map
func ContainsDictionaryWord(s string, dictionary map[string]struct{}) bool {
	words := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})

	words = append(words, CamelCaseToWords(s)...)

	for _, word := range words {
		if _, exists := dictionary[word]; exists {
			return true
		}
	}

	return false
}

// Function to recursively convert interface{} to JSON-compatible types
func convertInterface(value interface{}) interface{} {
	switch v := value.(type) {
	case map[interface{}]interface{}:
		return convertMap(v)
	case []interface{}:
		return convertSlice(v)
	default:
		return v
	}
}

// Function to convert map[interface{}]interface{} to map[string]interface{}
func convertMap(m map[interface{}]interface{}) map[string]interface{} {
	newMap := make(map[string]interface{})
	for key, value := range m {
		strKey, ok := key.(string)
		if !ok {
			// Handle the case where the key is not a string
			// Here, we simply skip the key-value pair
			continue
		}
		newMap[strKey] = convertInterface(value)
	}
	return newMap
}

// Function to recursively convert slices
func convertSlice(s []interface{}) []interface{} {
	newSlice := make([]interface{}, len(s))
	for i, value := range s {
		newSlice[i] = convertInterface(value)
	}
	return newSlice
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

// CamelCaseToWords converts a camel case string into a list of words.
func CamelCaseToWords(s string) []string {
	var words []string
	runes := []rune(s)
	start := 0

	for i := 1; i < len(runes); i++ {
		if unicode.IsUpper(runes[i]) {
			words = append(words, string(runes[start:i]))
			start = i
		}
	}

	// Add the last word
	words = append(words, string(runes[start:]))

	return words
}

// Pick some lines from a line number.
func PickLinesInFile(filename string, line_no, count int) (lines []string) {
	datab, err := os.ReadFile(filename)
	u.CheckErr(err, "PickLinesInFile ReadFile")
	datalines := strings.Split(string(datab), "\n")
	max_lines := len(datalines)
	if count == 0 {
		count = 1
	}
	start_index := line_no
	if start_index >= max_lines {
		return
	}
	end_index := start_index + count
	if end_index > max_lines-1 {
		end_index = max_lines - 1
	}
	return datalines[start_index:end_index]
}

// ExtractTextBlock extract a text from two set regex patterns. The text started with the line matched start_pattern
// and when hit the match for end_pattern it will stop not including_endlines
func ExtractTextBlock(filename string, start_pattern, end_pattern []string) (block string, start_line_no int, end_line_no int, datalines []string) {
	datab, err := os.ReadFile(filename)
	u.CheckErr(err, "ExtractTextBlock ReadFile")
	datalines = strings.Split(string(datab), "\n")

	found_start, found_end := false, false
	all_lines_count := len(datalines)

	found_start, start_line_no, _ = SearchPatternListInStrings(datalines, start_pattern, 0, all_lines_count, 0)
	if found_start {
		if start_line_no == all_lines_count-1 {
			found_end, end_line_no = true, all_lines_count
		} else {
			found_end, end_line_no, _ = SearchPatternListInStrings(datalines, end_pattern, start_line_no+1, all_lines_count, 0)
		}
		if found_end {
			outputlines := datalines[start_line_no:end_line_no]
			return strings.Join(outputlines, "\n"), start_line_no, end_line_no, datalines
		}
	}
	return
}

// Extract a text block which contains marker which could be an int or a list of pattern. if it is an int it is the line number.
// First we get the text from the line number or search for a match to the marker pattern. If we found we will search upward (to index 0) for the
// upper_bound_pattern, and when found, search for the lower_bound_pattern. The marker should be in the middle
// Return the text within the upper and lower, but not including the lower bound. Also return the line number range
func ExtractTextBlockContains(filename string, upper_bound_pattern, lower_bound_pattern []string, marker interface{}) (block string, start_line_no int, end_line_no int, datalines []string) {
	datab, err := os.ReadFile(filename)
	u.CheckErr(err, "ExtractTextBlock ReadFile")
	datalines = strings.Split(string(datab), "\n")
	all_lines_count := len(datalines)
	marker_line_no := 0
	found_marker, found_upper, found_lower := false, false, false

	if marker_int, ok := marker.(int); ok {
		if marker_int < all_lines_count && marker_int >= 0 {
			found_marker, marker_line_no = true, marker_int
		}
	} else {
		found_marker, marker_line_no, _ = SearchPatternListInStrings(datalines, marker.([]string), 0, all_lines_count, 0)
	}
	// Here we should found_marker and marker_line_no
	if !found_marker {
		return "", 0, 0, []string{}
	}
	// Search upper
	if marker_line_no == 0 { // We are at the start already, take that as upper and ignore upper ptn
		found_upper, start_line_no = true, marker_line_no
	} else {
		if len(upper_bound_pattern) > 0 {
			found_upper, start_line_no, _ = SearchPatternListInStrings(datalines, upper_bound_pattern, marker_line_no, all_lines_count, -1)
		} else {
			found_upper, start_line_no = true, marker_line_no
		}
	}
	if !found_upper {
		// fmt.Fprintf(os.Stderr, "UPPER not found. Ptn: %v\n", upper_bound_pattern)
		return "", 0, 0, []string{}
	}
	// Search lower
	if marker_line_no == all_lines_count-1 { // already at the end
		found_lower, end_line_no = true, all_lines_count
	} else { // as the current marker_line_no already match the upper, we started to search from that + 1
		found_lower, end_line_no, _ = SearchPatternListInStrings(datalines, lower_bound_pattern, marker_line_no+1, all_lines_count, 0)
	}
	if !found_lower {
		// fmt.Fprintf(os.Stderr, "LOWER not found. Ptn: %v\n", lower_bound_pattern)
		return "", 0, 0, []string{}
	}
	return strings.Join(datalines[start_line_no:end_line_no], "\n"), start_line_no, end_line_no, datalines
}

// Given a list of string of regex pattern and a list of string, find the coninuous match in that input list and return the start line
// of the match and the line content
// max_line defined the maximum line to search; set to 0 to use the len of input lines which is full
// start_line is the line to start searching; set to 0 to start from begining
// start_line should be smaller than max_line
// direction is the direction of the search -1 is upward; otherwise is down
func SearchPatternListInStrings(datalines []string, pattern []string, start_line, max_line, direction int) (found_marker bool, start_line_no int, linestr string) {
	marker_ptn := []*regexp.Regexp{}
	for _, ptn := range pattern {
		marker_ptn = append(marker_ptn, regexp.MustCompile(ptn))
	}
	count_ptn_found := len(marker_ptn)
	if max_line == 0 {
		max_line = len(datalines)
	}
	step := 1
	if direction != 0 { // Allow caller to set the step
		step = direction
	}
datalines_Loop:
	for idx := start_line; idx < max_line && idx >= 0; idx = idx + step {
		line := datalines[idx]
		// fmt.Fprintf(os.Stderr, "line:%d|step:%d - %s\n", idx, step, line)
		if marker_ptn[0].MatchString(line) { // Found first one. Lets look forward count_ptn_found-1 lines and see we got match
			for i := 1; i < count_ptn_found; i++ {
				if idx+count_ptn_found-1 >= max_line { // -1 because we already move 1 to get idx.
					break datalines_Loop // Can not look forward - out of bound
				}
				if !marker_ptn[i].MatchString(datalines[idx+i]) {
					continue datalines_Loop
				}
			}
			found_marker, start_line_no = true, idx
			linestr = datalines[idx]
			return
		}
	}
	return
}

// ExtractLineInLines will find a line match a pattern with capture (or not). The pattern is in between a start pattern and end pattern to narrow down
// search range. Return the result of FindAllStringSubmatch func of the match line
// This is simpler as it does not support multiple pattern as a marker like the other func eg ExtractTextBlockContains so input should be small
// and pattern match should be unique. Use the other function to devide it into small range and then use this func.
func ExtractLineInLines(blocklines []string, start, line, end string) [][]string {
	p0, p1, p3 := regexp.MustCompile(start), regexp.MustCompile(line), regexp.MustCompile(end)
	found_start, found, found_end := false, false, false
	var _l string
	for _, _l = range blocklines {
		if !found_start {
			found_start = p0.MatchString(_l)
		}
		if !found_end {
			found_end = p3.MatchString(_l)
		}
		if !found_start {
			continue
		} else {
			if !found_end {
				found = p1.MatchString(_l)
			} else {
				break
			}
		}
		if found {
			break
		}
	}
	if found {
		return p1.FindAllStringSubmatch(_l, -1)
	} else {
		return nil
	}
}

// SplitTextByPattern splits a multiline text into sections based on a regex pattern.
// If includeMatch is true, the matching lines are included in the result.
// pattern should a multiline pattern like `(?m)^Header line.*`
func SplitTextByPattern(text, pattern string, includeMatch bool) []string {
	re := regexp.MustCompile(pattern)
	sections := []string{}

	switch includeMatch {
	case true:
		matches := re.FindAllStringIndex(text, -1)
		start := 0
		for _, match := range matches {
			if start < match[0] {
				_t := strings.TrimSpace(text[start:match[0]])
				if _t != "" {
					sections = append(sections, _t)
				}
				start = match[0]
			}
		}
		sections = append(sections, text[start:]) // Capture the remaining part of the text
	case false:
		splitText := re.Split(text, -1)
		for _, part := range splitText {
			part = strings.TrimSpace(part)
			if part != "" {
				sections = append(sections, part)
			}
		}
	}
	return sections
}

// Edit line in a set of lines using simple regex and replacement
func LineInLines(datalines []string, search_pattern string, replace string) (output []string) {
	search_pattern_ptn := regexp.MustCompile(search_pattern)
	for i := 0; i < len(datalines); i++ {
		datalines[i] = search_pattern_ptn.ReplaceAllString(datalines[i], replace)
	}
	return datalines
}

// Find a block text matching and replace content with replText. Return the old text block
func BlockInFile(filename string, upper_bound_pattern, lower_bound_pattern []string, marker any, replText string, keepBoundaryLines bool, backup bool) (oldBlock string) {
	block, start_line_no, end_line_no, datalines := ExtractTextBlockContains(filename, upper_bound_pattern, lower_bound_pattern, marker)
	fstat, err := os.Stat(filename)
	if errors.Is(err, fs.ErrNotExist) {
		panic("[ERROR]BlockInFile File " + filename + " doesn't exist\n")
	}
	var upPartLines, downPartLines []string
	if keepBoundaryLines {
		upPartLines = datalines[0 : start_line_no+1]
		downPartLines = datalines[end_line_no:]
	} else {
		upPartLines = datalines[0:start_line_no]
		downPartLines = datalines[end_line_no+1:]
	}
	if backup {
		os.WriteFile(filename+".bak", []byte(strings.Join(datalines, "\n")), fstat.Mode())
	}
	os.WriteFile(filename, []byte(strings.Join(upPartLines, "\n")+"\n"+replText+"\n"+strings.Join(downPartLines, "\n")), fstat.Mode())
	return block
}
