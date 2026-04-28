package lib

import (
	"testing"

	u "github.com/sunshine69/golang-tools/utils"
)

func TestValidateYamlFile(t *testing.T) {
	o := map[string]any{}
	ValidateYamlFile("ansible/ansible-deploy-common/play.yaml", &o)
	println(u.JsonDump(o, ""))
}

func TestValidateYamlDir(t *testing.T) {
	o := map[string]any{}
	ValidateYamlDir("ansible/inventory", &o)
	println(u.JsonDump(o, ""))
}
