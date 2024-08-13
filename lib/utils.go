package lib

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	u "github.com/sunshine69/golang-tools/utils"
	"github.com/tidwall/gjson"
	"gopkg.in/ini.v1"
	"gopkg.in/yaml.v3"
)

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

func SliceToMap(slice []string) map[string]interface{} {
	set := make(map[string]interface{})
	for _, element := range slice {
		set[element] = ""
	}
	return set
}

func ItemExists(item string, set map[string]interface{}) bool {
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
	Path          string
	Regexp        string
	Search_string string
	State         string
	Backup        bool
}

func NewLineInfileOpt(opt map[string]interface{}) *LineInfileOpt {
	o := LineInfileOpt{
		Insertafter:   "",
		Insertbefore:  "",
		Line:          "",
		Regexp:        "",
		Search_string: "",
		State:         "present",
		Backup:        true,
	}
	for k, v := range opt {
		switch k {
		case "Insertafter", "insertafter":
			o.Insertafter = v.(string)
		case "Insertbefore", "insertbefore":
			o.Insertbefore = v.(string)
		case "Line", "line":
			o.Line = v.(string)
		case "Path", "path":
			o.Path = v.(string)
		case "Regexp", "regexp":
			o.Regexp = v.(string)
		case "Search_string", "search_string":
			o.Search_string = v.(string)
		case "State", "state":
			o.State = v.(string)
		case "Backup", "backup":
			o.Backup = v.(string) == "yes"
		default:
			panic("[ERROR] NewLineInfileOpt unknown option " + k)
		}
	}
	return &o
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
	if opt.Search_string != "" {
		search_string_found, line_exist_idx := true, map[int]interface{}{}
		index_list := []int{}
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
				datalines[last] = optLineB
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
		matches := [][]byte{}
		line_exist_idx := map[int]interface{}{}

		for idx, lineb := range datalines {
			matches = regex_ptn.FindSubmatch(lineb)
			if len(matches) > 0 || matches != nil {
				index_list = append(index_list, idx)
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
				for i, submatch := range matches {
					if submatch != nil {
						placeholder := fmt.Sprintf("$%d", i)
						optLineB = bytes.Replace(optLineB, []byte(placeholder), submatch, -1)
					}
				}
				datalines[last] = optLineB
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
func IsLikelyPasswordOrToken(value, check_mode string) bool {
	// Check length
	if len(value) < 8 || len(value) > 64 {
		return false
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
	const entropyThreshold = 3.0
	if entropy := calculateEntropy(value); entropy < entropyThreshold {
		return false
	}

	switch check_mode {
	case "letter":
		return hasUpper && hasLower
	case "digit":
		return hasDigit
	case "letter-digit":
		return hasUpper && hasLower && hasDigit
	case "letter-digit-word":
		dict, err := loadDictionary("words.txt", 0)
		u.CheckErr(err, "IsLikelyPasswordOrToken loadDictionary")
		if containsDictionaryWord(value, dict) {
			return false
		}
		return hasUpper && hasLower && hasDigit
	default:
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
func containsDictionaryWord(s string, dictionary map[string]struct{}) bool {
	words := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})

	for _, word := range words {
		if _, exists := dictionary[word]; exists {
			return true
		}
	}
	return false
}
