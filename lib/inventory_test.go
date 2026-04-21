package lib

import (
	"testing"

	u "github.com/sunshine69/golang-tools/utils"
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
	inv := ParseInventoryDirAll("../../go-ansible/test/inventory") // This parse the generator and ini format
	// inv := u.Must(ParseInventoryDir("../../go-ansible/test/inventory")) // Only ini format
	inv.ParseAllInventoryVars() // Get all vars in
	println(u.JsonDump(inv, ""))
	devhost := inv.MatchHost(`dev`)
	println("Matched host: ", u.JsonDump(devhost, ""))
	println(u.JsonDump(inv.Hosts[devhost[0]].Vars, ""))
	println("Matched group: ", u.JsonDump(inv.MatchGroup(`dev`), ""))

}
