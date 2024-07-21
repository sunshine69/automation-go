package lib

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

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

type LineInfileOpt struct {
	Attributes    string
	Backrefs      bool
	Backup        bool
	Create        bool
	Firstmatch    bool
	Group         string
	Insertafter   string
	Insertbefore  string
	Line          string
	Mode          string
	Owner         string
	Path          string
	Regexp        string
	Search_string string
	State         string
	Validate      string
}

// Simulate ansible lineinfile module. Not yet implemented
func LineInFile(filepath, pattern, replacement string, opt *LineInfileOpt) {
	fmt.Println("TODO")
}

// Given a key as string, may have dot like objecta.field_a.b. and a map[string]interface{}
// check if the map has the key path point to a non nil value; return true if value exists otherwise
func validateAKeyWithDotInAmap(key string, vars map[string]interface{}) bool {
	jsonB := u.JsonDumpByte(vars, "")
	r := gjson.GetBytes(jsonB, key)
	return r.Exists()
}

// Validate helm template. Pretty simple for now, not assess the set new var directive or include
// directive or long access var within range etc.
// Trivy and helm lint with k8s validation should cover that job
// This only deals with when var is not defined, helm content rendered as empty string.
// Walk throught the template, search for all string pattern with {{ .Values.<XXX> }} -
// then extract the var name.
// Load the helm values files into map, merge them and check the var name (or path access) in there.
// If not print outout error
// If there is helm template `if` statement to test the value then do not fail
func HelmChartValidation(chartPath string, valuesFile []string) bool {
	vars := map[string]interface{}{}
	for _, fn := range valuesFile {
		if ib, err := os.ReadFile(fn); err == nil {
			err = yaml.Unmarshal(ib, vars)
			u.CheckErr(err, "HelmChartValidation yaml.Unmarshal")
		} else {
			panic(fmt.Sprintf("[ERROR] loading values file %s - %s\n", fn, err.Error()))
		}
	}

	valuesPtn := regexp.MustCompile(`\{\{[\-]{0,1}[\s]*\.Values\.([^\s\}]+)[\s]*[\-]{0,1}\}\}`)
	valuesInIfStatementPtn := regexp.MustCompile(`\{\{[\-]{0,1}[\s]*if[\s]+\.Values\.([^\s\}]+)[\s]*[\-]{0,1}\}\}`)
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

		findResIfB := valuesInIfStatementPtn.FindAllSubmatch(fcontentb, -1)
		tempListIfMap := map[string]interface{}{}
		for _, res := range findResIfB {
			tempListIfMap[string(res[1])] = nil
		}

		findResB := valuesPtn.FindAllSubmatch(fcontentb, -1)
		tempList := []string{}
		for _, res := range findResB {
			_v := string(res[1])
			if _, ok := tempListIfMap[_v]; !ok {
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
