package main

import (
	"fmt"
	"os"

	"github.com/spf13/pflag"
	lib "github.com/sunshine69/automation-go/lib"
	u "github.com/sunshine69/golang-tools/utils"
)

func main() {
	optFlag := pflag.NewFlagSet("opt", pflag.ExitOnError)

	inserfater := optFlag.StringP("insertafter", "a", "", "insertafter")
	insertbefore := optFlag.StringP("insertbefore", "b", "", "insertbefore")
	line := optFlag.StringP("line", "l", "", "Line to insert")
	// filename := pflag.StringP("file", "f", "", "Filename or path")
	regexptn := optFlag.StringP("regexp", "r", "", "regexp")
	search_string := optFlag.StringP("search_string", "s", "", "search string")
	backup := optFlag.Bool("backup", true, "backup")
	cmd_mode := optFlag.StringP("cmd", "c", "lineinfile", "Command; choice lineinfile, search_replace")
	state := optFlag.String("state", "present", "state; choices: present, absent")

	filename := os.Args[1]
	optFlag.Usage = func() {
		fmt.Printf("Usage: %s [filename/path] [opt]\n", os.Args[0])
		optFlag.PrintDefaults()
	}
	optFlag.Parse(os.Args[1:])

	switch *cmd_mode {
	case "lineinfile":
		opt := lib.LineInfileOpt{
			Insertafter:   *inserfater,
			Insertbefore:  *insertbefore,
			Line:          *line,
			Regexp:        *regexptn,
			Search_string: *search_string,
			State:         *state,
			Backup:        *backup,
		}
		fmt.Println(u.JsonDump(opt, "  "))
		err, changed := lib.LineInFile(filename, &opt)
		u.CheckErr(err, "")
		fmt.Printf("changed: %v\n", changed)
	case "search_replace":
		if *regexptn == "" {
			panic("option regexp (r) is required")
		}
		lib.SearchReplaceFile(filename, *regexptn, *line, -1, *backup)
	}

}
