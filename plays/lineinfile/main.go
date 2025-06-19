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

var (
	// Build cmd so we have the version into the binary - eg in fish shell
	// env CGO_ENABLED=0 go build -trimpath -ldflags="-X main.version=v1.0.1+"(date +'%Y%m%d')" -X main.buildTime="(date +'%Y-%m-%d_%H:%M:%S')" -extldflags=-static -w -s" --tags "osusergo,netgo" -o lineinfile-linux-amd64 plays/lineinfile/main.go
	version   string // Will hold the version number
	buildTime string // Will hold the build time
)

func printVersionBuildInfo() {
	fmt.Printf("Version: %s\nBuild time: %s\n", version, buildTime)
}

func main() {
	optFlag := pflag.NewFlagSet("opt", pflag.ExitOnError)
	insertafter := optFlag.StringP("insertafter", "a", "", "insertafter. In blockinfile it is a json list of regex string used as upperBound")
	insertbefore := optFlag.StringP("insertbefore", "b", "", "insertbefore. In blockinfile it is a json list of regex string used as lowerBound")
	line := optFlag.StringP("line", "l", "", "Line(s) to insert. Can contains regex capture if your regex option has it - like $1, $2 etc..")
	regexptn := optFlag.StringP("regexp", "r", "", "regexp to match for mode regex_search, Can contains group capture. In blockinfile mode it is a json list of regex string used as the marker")
	search_string := optFlag.StringP("search_string", "s", "", "search string. This is used in non regex mode")
	backup := optFlag.Bool("backup", true, "backup")
	erroIfNoChanged := optFlag.Bool("errorifnochange", false, "Exit with error status if no changed detected")
	cmd_mode := optFlag.StringP("cmd", "c", "lineinfile", `Command; choices:
lineinfile - insert or make sure the line exist matching the search_string if set or insert new one. This is default.
search_replace - insert or make sure the line exist matching the regex pattern
blockinfile - make sure the block lines exists in file
  In this mode we will take these option - insertafter, insertbefore and regexp as upperBound, lowerBound and marker to call the function. They should be a json list of regex string if defined or empty
  Example to replace the ansible vault in bash shell
    export e="$(ansible-vault encrypt_string 'somepassword' | grep -v 'vault')"
    lineinfile tmp/input.yaml -c blockinfile -a '["^key2\\: \\!vault \\|$"]' -r '["^[\\s]+\\$ANSIBLE_VAULT.*$"]' -b '["^[\\s]*([^\\d]*|\\n|EOF)$"]' --line "$e"
	It will replace the vault data only, keeping the like 'key2: !vault |' intact
	To be reliable for success you should pass -a, -b, -r properly and ensure they matches uniquely so the program can
	detect block boundary correctly.
`)
	state := optFlag.String("state", "present", `state; choices:
present         - line present.
absent          - remove line
keepboundary    - same as present used in blockinfile; the block itself does not contain the upper and lower
boundary. In other word, do not touch the upper and lower marker, just replace text block in between.
This is the default mode
If you do not want set state=includeboundary
includeboundary    - The inverse - that is the block of text we replace include upper and lower string marker.
print           - Print only print lines of matches but do nothing`)
	grep := optFlag.StringP("grep", "g", "", "Simulate grep cmd. It will set state to print and take -r for pattern to grep")
	filename_ptn := optFlag.StringP("fptn", "f", ".*", "Filename regex pattern")
	exclude := optFlag.StringP("exclude", "e", "", "Exclude file name pattern")
	defaultExclude := optFlag.StringP("defaultexclude", "d", `^(\.git|.*\.zip|.*\.gz|.*\.xz|.*\.bz2|.*\.zst|.*\.7z|.*\.dll|.*\.iso|.*\.bin|.*\.tar|.*\.exe)$`, "Default exclude pattern. Set it to empty string if you need to")
	skipBinary := optFlag.BoolP("skipbinary", "y", false, "Skip binary file")
	debug := optFlag.Bool("debug", false, "Enable debugging")

	if len(os.Args) < 2 {
		panic(`{"error": "missing file argument. Run with option -h for help"}`)
	}
	file_path := os.Args[1]
	optFlag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [filename/path] [opt]\n", os.Args[0])
		optFlag.PrintDefaults()
		os.Exit(0)
	}
	optFlag.Parse(os.Args[1:])

	if file_path == "version" {
		printVersionBuildInfo()
		os.Exit(0)
	}

	if *grep != "" {
		*state = "print"
		*regexptn = *grep
		*search_string = ""
		*backup = false
		*line = ""
	}

	opt := u.LineInfileOpt{
		Insertafter:   *insertafter,
		Insertbefore:  *insertbefore,
		Line:          *line,
		Regexp:        *regexptn,
		Search_string: *search_string,
		State:         *state,
		Backup:        *backup,
	}
	if *debug {
		fmt.Fprintf(os.Stderr, "cmd: %s\nOpt: %s\n", *cmd_mode, u.JsonDump(opt, "  "))
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
	isthereChange := false

	err := filepath.Walk(file_path, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
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
				output[path] = []interface{}{changed, err}
				if !isthereChange && changed {
					isthereChange = true
				}
			case "search_replace":
				if *regexptn == "" {
					panic(`{"error": "option regexp (r) is required"}`)
				}
				count := u.SearchReplaceFile(path, *regexptn, *line, -1, *backup)
				output[path] = []interface{}{count, nil}
				if !isthereChange && count > 0 {
					isthereChange = true
				}
			case "blockinfile":
				if *state == "present" {
					*state = "keepboundary"
				}
				upperBound, lowerBound, marker := []string{}, []string{}, []string{}
				u.CheckErr(json.Unmarshal([]byte(*insertafter), &upperBound), "Unmarshal upperBound "+*insertafter)
				u.CheckErr(json.Unmarshal([]byte(*insertbefore), &lowerBound), "Unmarshal lowerBound "+*insertbefore)
				u.CheckErr(json.Unmarshal([]byte(*regexptn), &marker), "Unmarshal marker "+*regexptn)
				oldblock := u.BlockInFile(path, upperBound, lowerBound, marker, *line, *state == "keepboundary", false)
				fmt.Fprintln(os.Stderr, oldblock)
				if oldblock != "" {
					output[path] = []interface{}{1, nil}
					if !isthereChange {
						isthereChange = true
					}
				}
			default:
				fmt.Fprintln(os.Stderr)
				panic(`{"error": "Unknown command "` + *cmd_mode + `"}`)
			}
		}
		return nil
	})
	u.CheckErrNonFatal(err, "main")
	if *state != "print" {
		fmt.Printf("%s\n", u.JsonDump(output, "  "))
		if *erroIfNoChanged && !isthereChange {
			os.Exit(1)
		}
	}
}
