package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"

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
	state := optFlag.String("state", "present", "state; choices: present, absent, print. Print only print lines of matches but do nothing")
	grep := optFlag.StringP("grep", "g", "", "Simulate grep cmd. It will set state to print and take -r or -s for pattern/string to grep")
	filename_ptn := optFlag.StringP("fptn", "f", ".*", "Filename regex pattern")
	exclude := optFlag.StringP("exclude", "e", "", "Exclude file name pattern")
	defaultExclude := optFlag.StringP("defaultexclude", "d", `^(\.git|.*\.zip|.*\.gz|.*\.xz|.*\.bz2|.*\.zstd|.*\.7z)$`, "Default exclude pattern. Clean it if you need to control")

	file_path := os.Args[1]
	optFlag.Usage = func() {
		fmt.Printf("Usage: %s [filename/path] [opt]\n", os.Args[0])
		optFlag.PrintDefaults()
	}
	optFlag.Parse(os.Args[1:])

	if *grep != "" {
		*state = "print"
		*regexptn = *grep
		*search_string = ""
		*backup = false
		*line = ""
	}

	opt := lib.LineInfileOpt{
		Insertafter:   *inserfater,
		Insertbefore:  *insertbefore,
		Line:          *line,
		Regexp:        *regexptn,
		Search_string: *search_string,
		State:         *state,
		Backup:        *backup,
	}
	// fmt.Printf("cmd: %s\nOpt: %s\n", *cmd_mode, u.JsonDump(opt, "  "))

	filename_regexp := regexp.MustCompile(*filename_ptn)
	excludePtn := regexp.MustCompile(*exclude)
	if *exclude == "" {
		excludePtn = nil
	}
	defaultExcludePtn := regexp.MustCompile(*defaultExclude)
	if *defaultExclude == "" {
		defaultExcludePtn = nil
	}
	output := map[string][]interface{}{}

	err := filepath.Walk(file_path, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && ((excludePtn != nil && excludePtn.MatchString(info.Name())) || (defaultExcludePtn != nil && defaultExcludePtn.MatchString(info.Name()))) {
			return filepath.SkipDir
		}
		// Check if the file matches the pattern
		fname := filepath.Base(path)
		if !info.IsDir() && filename_regexp.MatchString(fname) {
			if excludePtn != nil {
				if excludePtn.MatchString(fname) {
					return nil
				}
			}
			switch *cmd_mode {
			case "lineinfile":
				err, changed := lib.LineInFile(path, &opt)
				u.CheckErr(err, "")
				output[path] = append(output[path], []interface{}{err, changed})
			case "search_replace":
				if *regexptn == "" {
					panic("option regexp (r) is required")
				}
				count := lib.SearchReplaceFile(path, *regexptn, *line, -1, *backup)
				output[path] = append(output[path], []interface{}{nil, count})
			default:
				panic("Unknown command " + *cmd_mode)
			}

		}
		return nil
	})
	u.CheckErr(err, "")
	if *state != "print" {
		fmt.Printf("output: %s\n", u.JsonDump(output, "  "))
	}
}
