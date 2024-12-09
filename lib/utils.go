package lib

import (
	"bufio"
	"fmt"
	"io/fs"
	"math"
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

// Validate a yaml file and load it into a map
func IncludeVars(filename string) map[string]interface{} {
	m := make(map[string]interface{})
	ValidateYamlFile(filename, &m)
	return m
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
		fcontentb := u.Must(os.ReadFile(path))
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
	data := u.Must(os.ReadFile(yaml_file))
	if yamlobj == nil {
		t := map[string]interface{}{}
		yamlobj = &t
	}
	err := yaml.Unmarshal(data, &yamlobj)
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
func IsLikelyPasswordOrToken[W string | map[string]struct{}](value, check_mode string, words_source W, word_len int, entropy_threshold float64) bool {
	// Check length
	if len(value) < 6 || len(value) > 64 {
		// fmt.Printf("[WARN] Skipping %s as len is not > 8 and < 64\n", value)
		return false
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
	hasWord := false
	var word_dict map[string]struct{} = nil

	detectHasWord := func(word_dict map[string]struct{}) bool {
		anywords_source := any(words_source)
		if words_file_path, ok := anywords_source.(string); ok {
			if words_file_path == "" {
				words_file_path = "words.txt"
			}
			if word_dict == nil {
				word_dict = u.Must(LoadWordDictionary(words_file_path, word_len))
			}
		} else if _word_dict, ok := anywords_source.(map[string]struct{}); ok {
			word_dict = _word_dict
		} else {
			panic("word_source is nil and we need it\n")
		}
		return ContainsDictionaryWord(value, word_dict)
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
		hasWord = detectHasWord(word_dict)
		if hasWord {
			return false
		}
		return hasUpper && hasLower
	case "letter+digit+word":
		hasWord = detectHasWord(word_dict)
		if hasWord {
			return false
		}
		return hasUpper && hasLower && hasDigit
	default:
		hasWord = detectHasWord(word_dict)
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
func LoadWordDictionary(filename string, word_len int) (map[string]struct{}, error) {
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
	words = append(words, u.CamelCaseToWords(s)...)
	for _, word := range words {
		if _, exists := dictionary[word]; exists {
			return true
		}
	}
	return false
}
