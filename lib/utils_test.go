package lib

import (
	"fmt"
	"os"
	"testing"
)

var project_dir string

func init() {
	project_dir, _ = os.Getwd()
}

func TestAddhoc(t *testing.T) {
	// varAnsible := ParseVarAnsibleNext(project_dir+"/../work/azure-devops/vars-ansible.yaml", project_dir)
	HelmChartValidation("/home/stevek/src/helm_playground-1_v1/", []string{
		"//home/stevek/src/helm_playground-1_v1/values.yaml",
	})
	// tmplStr := `{{ "test".upper() }}`
	// o := TemplateString(tmplStr, map[string]interface{}{})
	// fmt.Printf("%s\n", o)
	fmt.Println("Done test")
}
