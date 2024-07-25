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
	// HelmChartValidation("/home/sitsxk5/src/Sonic.Slingshot.Config-Tanzu/Tanzu/Helm/slingshot-srv-config", []string{
	// 	"/home/sitsxk5/tmp/helm-values.yaml",
	// 	"/home/sitsxk5/tmp/helm-values-1.yaml",
	// })
	o := MaskCredential(`secret_key: "rterte somrtthing: broken"`)
	fmt.Println(o)

	fmt.Println("Done test")
}
