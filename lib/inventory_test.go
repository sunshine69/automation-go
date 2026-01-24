package lib

import (
	"testing"

	u "github.com/sunshine69/golang-tools/utils"
)

func TestParseInventoryDir(t *testing.T) {
	inventory := ParseInventoryDir("/mnt/nfs-data/stevek-src/go-automation/inventory")
	println(u.JsonDump(inventory, ""))
	hosts := u.Must(inventory.MatchHosts("*uat*"))
	for hname, h := range hosts {
		println("***** HOST NAME: " + hname + "  *********")
		Vars := u.Must(FlattenAllVars(u.StringMapToAnyMap(h.Vars)))
		println(u.JsonDump(Vars, ""))
	}
}
