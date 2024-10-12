package lib

import (
	"fmt"
	"os"
	"strings"
	"testing"

	u "github.com/sunshine69/golang-tools/utils"
	"gopkg.in/yaml.v3"
)

var project_dir string

func init() {
	project_dir, _ = os.Getwd()
}

func TestAddhoc(t *testing.T) {
	text := `Header 1
	This is some content
	for the first section.	
	Header 2
	This is some content
	for the second section.	

	Header 3
	This is some content
	for the third section.`

	sections := SplitTextByPattern(text, `(?m)Header.*`, false)
	for idx, r := range sections {
		fmt.Printf("Rows %d\n%s\n", idx+1, r)
	}

	fmt.Println("Done test")
}

func TestLineinfile(t *testing.T) {
	err, changed := LineInFile("../tmp/tests.yaml", NewLineInfileOpt(&LineInfileOpt{
		// Regexp:     `v1.0.1(.*)`,
		Search_string: "This is new line",
		Line:          "This is new line to be reaplced at line 4",
		// ReplaceAll: true,
	}))
	u.CheckErr(err, "Error")
	fmt.Println(changed)
}

func TestExtractBlock(t *testing.T) {
	o, _, _, _ := ExtractTextBlock("../tmp/tests.yaml", []string{
		`- name: "Run trivy to scan Dockerfile"`,
	},
		[]string{`msg: \|`})
	// o = `MYVAR:\n` + o
	fmt.Println(o)
	o1 := []map[string]interface{}{}
	u.CheckErr(yaml.Unmarshal([]byte(o), &o1), "ERR")
	fmt.Printf("%s\n", u.JsonDump(o1, "  "))
}

func TestExtractBlockContains(t *testing.T) {
	fmt.Println("****** INTEGER *******")
	o, _, _, _ := ExtractTextBlockContains("../tmp/tests.yaml", []string{`- [^\s]+:[ ]?[^\s]*`}, []string{`- [^\s]+:[ ]?[^\s]*`}, 0)
	fmt.Printf("'%s'\n", o)
	o1 := []map[string]interface{}{}
	u.CheckErr(yaml.Unmarshal([]byte(o), &o1), "ERR")
	fmt.Printf("%s\n", u.JsonDump(o1, "  "))

	fmt.Println("\n****** PTN *******")
	o, _, _, _ = ExtractTextBlockContains("../tmp/tests.yaml", []string{`- [^\s]+:[ ]?[^\s]*`}, []string{`- [^\s]+:[ ]?[^\s]*`}, []string{`helm_chart_resource_fact: "{{ helm_chart_resource }}"`})
	fmt.Printf("'%s'\n", o)
	o1 = []map[string]interface{}{}
	u.CheckErr(yaml.Unmarshal([]byte(o), &o1), "ERR")
	fmt.Printf("%s\n", u.JsonDump(o1, "  "))

	fmt.Println("\n****** PTN *******")
	o, _, _, _ = ExtractTextBlockContains("../tmp/tests.yaml", []string{`- when: "build_enabled_docker or build_enabled_helm"`}, []string{`msg: "{{ fail_msg }}"`}, []string{`msg: "{{ fail_msg }}"`})
	fmt.Printf("'%s'\n", o)
	// o1 = []map[string]interface{}{}
	// u.CheckErr(yaml.Unmarshal([]byte(o), &o1), "ERR")
	// fmt.Printf("%s\n", u.JsonDump(o1, "  "))

}

func TestPickLinesInFile(t *testing.T) {
	fmt.Println(strings.Join(PickLinesInFile("../tmp/tests.yaml", 70, 1), "\n"))
}

func TestLineInLines(t *testing.T) {
	o, _, _, _ := ExtractTextBlockContains("../tmp/tests.yaml", []string{`- [^\s]+:[ ]?[^\s]*`}, []string{`- [^\s]+:[ ]?[^\s]*`}, []string{`helm_chart_resource_fact: "{{ helm_chart_resource }}"`})
	fmt.Printf("'%s'\n", o)
	r := LineInLines(strings.Split(o, "\n"), `- set_fact:`, `- ansible.builtin.set_fact: `)
	fmt.Printf("'%s'\n", strings.Join(r, "\n"))
}
