package lib

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

var project_dir string

func init() {
	project_dir, _ = os.Getwd()
}

func TestAddhoc(t *testing.T) {
	// varAnsible := ParseVarAnsibleNext(project_dir+"/../work/azure-devops/vars-ansible.yaml", project_dir)
	// HelmChartValidation("/home/stevek/src/helm_playground-1_v1/", []string{
	// 	"//home/stevek/src/helm_playground-1_v1/values.yaml",
	// })
	// tmplStr := `{{ "test".upper() }}`
	// o := TemplateString(tmplStr, map[string]interface{}{})
	// fmt.Printf("%s\n", o)
	// str := "Let freedom ring from the mighty mountains of New York. Let freedom ring from the heightening Alleghenies of Pennsylvania. Let freedom ring from the snow-capped Rockies of Colorado. Let freedom ring from the curvaceous slopes of California."
	// counter := 1
	// repl := func(match string) string {
	// 	old := counter
	// 	counter++
	// 	if old != 1 {
	// 		return fmt.Sprintf("[%d] %s%d", old, match, old)
	// 	}
	// 	return fmt.Sprintf("[%d] %s", old, match)
	// }
	// re := regexp.MustCompile("Let freedom")
	// str2 := re.ReplaceAllStringFunc(str, repl)
	// fmt.Println(str2)

	// err, changed := LineInFile("/home/stevek/tmp/hosts", &LineInfileOpt{
	// 	// Insertafter: "NOT FOUND",
	// 	// Search_string: "127.0.0.1",
	// 	Regexp: `127\.0\.[01]\.1`,
	// 	// Line:  "new data here",
	// 	State: "absent",
	// })
	// u.CheckErr(err, "")
	// words, _ := loadDictionary("/tmp/words", 0)

	words := strings.FieldsFunc(strings.ToLower("i0/UPPPERCASE"), func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
	fmt.Printf("%v\n", words)

	fmt.Printf("Likely a password: %v\n", IsLikelyPasswordOrToken("i0.ElementRef", "letter+digit+word", "/tmp/words.txt", 2.5))
	// fmt.Println(changed)
	ptn := regexp.MustCompile(`(?i)['"]?(password|passwd|token|api_key|secret)['"]?[=:\s][\s]*?['"]?([^'"\s]+)['"]?`)
	matches := ptn.FindAllStringSubmatch(`token: i0.ElementRef`, -1)
	fmt.Printf("%q\n", matches)
	fmt.Println("Done test")
}
