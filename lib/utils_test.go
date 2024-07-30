package lib

import (
	"fmt"
	"os"

	"testing"

	u "github.com/sunshine69/golang-tools/utils"
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

	err, changed := LineInFile("/home/stevek/tmp/hosts", &LineInfileOpt{
		// Insertafter: "NOT FOUND",
		// Search_string: "127.0.0.1",
		Regexp: `127\.0\.[01]\.1`,
		// Line:  "new data here",
		State: "absent",
	})
	u.CheckErr(err, "")
	fmt.Printf("Likely a password: %v\n", IsLikelyPasswordOrToken("DUMMY", ""))
	fmt.Println(changed)
	fmt.Println("Done test")
}
