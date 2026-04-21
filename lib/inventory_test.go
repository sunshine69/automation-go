package lib

import (
	"os"
	"strings"
	"testing"

	u "github.com/sunshine69/golang-tools/utils"
	"gopkg.in/yaml.v3"
)

func TestParseInventoryDir(t *testing.T) {
	inventory := u.Must(ParseInventoryDir("/mnt/nfs-data/stevek-src/go-automation/inventory"))
	println(u.JsonDump(inventory, ""))

	for hname, h := range inventory.Hosts {
		println("***** HOST NAME: " + hname + "  *********")
		Vars := u.Must(FlattenAllVars(h.Vars))
		println(u.JsonDump(Vars, ""))
	}
}

func TestParseInvetoryAll(t *testing.T) {
	// ParseInventoryXXX only parse inventory, group hosts
	// Vars is called later on to populate the vars
	inv := ParseInventoryDirAll("../../go-automation/inventory") // This parse the generator and ini format
	// inv := u.Must(ParseInventoryDir("../../go-ansible/test/inventory")) // Only ini format
	inv.ParseAllInventoryVars() // Get all vars in
	println(u.JsonDump(inv, ""))
	devhost := inv.MatchHost(`dev`)
	println("Matched host: ", u.JsonDump(devhost, ""))
	println(u.JsonDump(inv.Hosts[devhost[0]].Vars, ""))
	println("Matched group: ", u.JsonDump(inv.MatchGroup(`dev`), ""))

}

func TestGenerateINIFromConfig(t *testing.T) {
	invConfig := GeneratorConfig{}
	u.CheckErr(yaml.Unmarshal(u.Must(os.ReadFile("../../go-automation/inventory/hosts.yaml")), &invConfig), "")
	iniContent := GenerateIniFromConfig(&invConfig)
	println("[DEBUG] ini content:\n", iniContent)
	// Example of creating inventory steps. First create empty inv
	inv := NewInventory("../../go-automation/inventory")
	println("[DEBUG] Empty inv:\n" + u.JsonDump(inv, ""))
	// Parse ini string. You can parse a file by calling ParseInventory(file_path, ). After that we got group and hosts
	u.CheckErr(ParseInventory(strings.NewReader(iniContent), inv), "")
	println("[DEBUG] Inv after parsing:\n" + u.JsonDump(inv, ""))
	// Now parse group vars. This only tate the dir group_vars/<files>
	inv.ParseGroupVars("")
	println("[DEBUG] Parse inv after parse group vars:\n" + u.JsonDump(inv, ""))
	// Merge vars from group to hosts. This should preserve existing host vars
	inv.MergeVars()
	println("[DEBUG] Parse inv after merge vars:\n" + u.JsonDump(inv, ""))
	// Next parse the var in the inventory file or reader. This case is reader as we dont have inventory file
	inv.ParseInventoryVars(strings.NewReader(iniContent))
	println("[DEBUG] Parse inv after parse inventory vars:\n" + u.JsonDump(inv, ""))
	// Next parse vars from host_vars/<files>
	inv.ParseHostVars("")
	println("[DEBUG] Parse inv after parse hosts vars:\n" + u.JsonDump(inv, ""))
}
