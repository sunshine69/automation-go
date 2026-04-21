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
	// inv := ParseInventoryDirAll("../../go-ansible/test/inventory")
	inv := u.Must(ParseInventoryDir("../../go-ansible/test/inventory"))
	inv.ParseAllInventory()
	println(u.JsonDump(inv, ""))
}
