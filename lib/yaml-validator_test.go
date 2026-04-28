package lib

import (
	"testing"

	u "github.com/sunshine69/golang-tools/utils"
)

func TestValidateYamlFile(t *testing.T) {
	o := ValidateYamlFile("ansible/ansible-deploy-common/play.yaml")
	println(u.JsonDump(o, ""))
}

func TestValidateYamlDir(t *testing.T) {
	o := map[string]any{}
	ValidateYamlDir("ansible/inventory")
	println(u.JsonDump(o, ""))
}

func TestIncludeVars(t *testing.T) {
	o := IncludeVars("azure-devops/vars-ansible.yaml")
	println(u.JsonDump(o, ""))
}
