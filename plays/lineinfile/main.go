package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"

	"github.com/spf13/pflag"
	u "github.com/sunshine69/golang-tools/utils"
)

func main() {
	optFlag := pflag.NewFlagSet("opt", pflag.ExitOnError)
	inserfater := optFlag.StringP("insertafter", "a", "", "insertafter. In blockinfile it is a json list of regex string used as upperBound")
	insertbefore := optFlag.StringP("insertbefore", "b", "", "insertbefore. In blockinfile it is a json list of regex string used as lowerBound")
	line := optFlag.StringP("line", "l", "", "Line(s) to insert. Can contains regex capture if your regex option has it - like $1, $2 etc..")
	// filename := pflag.StringP("file", "f", "", "Filename or path")
	regexptn := optFlag.StringP("regexp", "r", "", "regexp to match for mode regex_search, Can contains group capture. In blockinfile mode it is a json list of regex string used as the marker")
	search_string := optFlag.StringP("search_string", "s", "", "search string. This is used in non regex mode")
	backup := optFlag.Bool("backup", true, "backup")
	cmd_mode := optFlag.StringP("cmd", "c", "lineinfile", `Command; choices:
lineinfile - insert or make sure the line exist matching the search_string if set or insert new one
search_replace - insert or make sure the line exist matching the regex pattern
blockinfile - make sure the block lines exists in file`)
	state := optFlag.String("state", "present", `state; choices:
present         - line present.
absent          - remove line
includeboundary - same as present used in blockinfile; the block itself contain the upper and lower boundary.
In blockinfile mode this is the default. If you do not want set state=keepboundary
keepboundary    - See above
print           - Print only print lines of matches but do nothing`)
	grep := optFlag.StringP("grep", "g", "", "Simulate grep cmd. It will set state to print and take -r for pattern to grep")
	filename_ptn := optFlag.StringP("fptn", "f", ".*", "Filename regex pattern")
	exclude := optFlag.StringP("exclude", "e", "", "Exclude file name pattern")
	defaultExclude := optFlag.StringP("defaultexclude", "d", `^(\.git|.*\.zip|.*\.gz|.*\.xz|.*\.bz2|.*\.zstd|.*\.7z|.*\.dll|.*\.iso|.*\.bin|.*\.tar|.*\.exe)$`, "Default exclude pattern. Set it to empty string if you need to")
	skipBinary := optFlag.BoolP("skipbinary", "y", false, "Skip binary file")
	debug := optFlag.Bool("debug", false, "Enable debugging")

	if len(os.Args) < 2 {
		panic("missing file argument. Run with option -h for help")
	}
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

	opt := u.LineInfileOpt{
		Insertafter:   *inserfater,
		Insertbefore:  *insertbefore,
		Line:          *line,
		Regexp:        *regexptn,
		Search_string: *search_string,
		State:         *state,
		Backup:        *backup,
	}
	if *debug {
		fmt.Printf("cmd: %s\nOpt: %s\n", *cmd_mode, u.JsonDump(opt, "  "))
	}

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
			fmt.Println(err.Error())
			return nil
		}
		fname := info.Name()
		if info.IsDir() && ((excludePtn != nil && excludePtn.MatchString(fname)) || (defaultExcludePtn != nil && defaultExcludePtn.MatchString(fname))) {
			return filepath.SkipDir
		}
		// Check if the file matches the pattern

		if !info.IsDir() && filename_regexp.MatchString(fname) && ((excludePtn == nil) || (excludePtn != nil && !excludePtn.MatchString(fname))) && ((defaultExcludePtn == nil) || (defaultExcludePtn != nil && !defaultExcludePtn.MatchString(fname))) {
			if *skipBinary {
				isbin, err := u.IsBinaryFileSimple(path)
				if (err == nil) && isbin {
					return nil
				}
			}
			switch *cmd_mode {
			case "lineinfile":
				err, changed := u.LineInFile(path, &opt)
				u.CheckErrNonFatal(err, "main lineinfile")
				output[path] = append(output[path], []interface{}{err, changed})
			case "search_replace":
				if *regexptn == "" {
					panic("option regexp (r) is required")
				}
				count := u.SearchReplaceFile(path, *regexptn, *line, -1, *backup)
				output[path] = append(output[path], []interface{}{nil, count})
			case "blockinfile":
				// In this mode we will take these option - insertafter, insertbefore and regexp as upperBound, lowerBound and marker to call the function. They should be a json list of regex string if defined or empty
				if *state == "present" || *state != "keepboundary" {
					*state = "includeboundary"
				}
				upperBound, lowerBound, marker := []string{}, []string{}, []string{}
				u.CheckErr(json.Unmarshal([]byte(*inserfater), &upperBound), "Unmarshal upperBound")
				u.CheckErr(json.Unmarshal([]byte(*insertbefore), &lowerBound), "Unmarshal lowerBound")
				u.CheckErr(json.Unmarshal([]byte(*regexptn), &marker), "Unmarshal marker")

				u.BlockInFile(path, upperBound, lowerBound, marker, *line, *state == "keepboundary", false)

			default:
				panic("Unknown command " + *cmd_mode)
			}
		}
		return nil
	})
	u.CheckErrNonFatal(err, "main 107")
	if *state != "print" {
		fmt.Printf("output: %s\n", u.JsonDump(output, "  "))
	}
}
