package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

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
	backup := optFlag.Bool("backup", false, "backup")
	erroIfNoChanged := optFlag.Bool("errorifnochange", false, "Exit with error status if no changed detected")
	cmd_mode := optFlag.StringP("cmd", "c", "lineinfile", `Command; choices:
lineinfile - insert or make sure the line exist matching the search_string if set or insert new one. This is default.

If file does not exist it will be created.

  - The simplest way to mimic sed is using option -r 'regex-string' -l 'new line content' - it will search the regex ptn and replace with 'new line content'. regex-string can have group capture and in the line content you can expand capture using $N where N is the capture group number. Be sure only run once as using capture like this is likely not idempotant as re-run it will keep adding text to the capture output

  The -r option wont add new line if teh regex not found. To add new line use option -s below. Thid is to make sure it is idempotant. You can add using -s first and then use -r to modify it if needed. Remember this tool can be run multiple times to lines stream editing of a file.

  - If you want to search raw string instead then use option -s instead of -r. it just replace the line contains the search string with new line. This is idempotant. If search string is not found line will be inserted based on the option -a (intertafter) or -b (insertbefore). If all not found or not supplied it will be inserted to the end of file. EOF and BOF is for end of file or begin of file respectively that can be used for option -a or -b.

  - note that you need to provide -r OR -s even it may not match it will add the line. Without -r or -s it wont do anything

  - note that this is single line editing. To perform search and replace for the whole file use the next command below

  - With the state=absent option -l is ignored. Only -s or -r to search for string or regex - it will remove all lines matched.

search_replace|replace - insert or make sure the line exist matching the regex pattern
  - note that it is multiline search thus regex anchor ^ and $ wont match. Pattern can have capture group and value of group is expanded in the line using $N where N is the group number.

blockinfile - make sure the block lines exists in file

  In this mode we will take these option - insertafter, insertbefore and regexp as upperBound, lowerBound and marker to call the function. They should be a json list of regex string if defined or empty

  Example to replace the ansible vault in bash shell
    export e="$(ansible-vault encrypt_string 'somepassword' | grep -v 'vault')"

	lineinfile tmp/input.yaml -c blockinfile -a '["^key2\\: \\!vault \\|$"]' -r '["^[\\s]+\\$ANSIBLE_VAULT.*$"]' -b '["^[\\s]*([^\\d]*|\\n|EOF)$"]' --line "$e"

	EOF is special string to allow return when reaching end of file and regarded as a match lower bound.

	It will replace the vault data only, keeping the like 'key2: !vault |' intact
	To be reliable for success you should pass -a, -b, -r properly and ensure they matches uniquely so the program can detect block boundary correctly.
`)
	state := optFlag.StringP("state", "S", "present", `state; choices:
present      - line | block present.
absent       - remove line

keepboundary - same as present used in blockinfile; the block itself does not contain the upper and lower
boundary. In other word, do not touch the upper and lower marker, just replace text block in between.
This is the default mode. If markers not found the block will be inserted *with* the boundary (option -a, -b)
If you do not want set state=includeboundary below.

includeboundary - The inverse - that is the block of text we replace include upper and lower string marker.

print           - Print only print lines of matches but do nothing

extract         - Only in blockinfile; it extract the text and return it the content, start line and end line. Run it and see the json it returns for further processing`)

	grep := optFlag.StringP("grep", "g", "", "Simulate grep cmd. It will set state to print and take -r for pattern to grep")
	filename_ptn := optFlag.StringP("fptn", "f", ".*", "Filename regex pattern")
	exclude := optFlag.StringP("exclude", "e", "", "Exclude file name pattern")
	defaultExclude := optFlag.StringP("defaultexclude", "d", `^(\.git|.*\.zip|.*\.gz|.*\.xz|.*\.bz2|.*\.zst|.*\.7z|.*\.dll|.*\.iso|.*\.bin|.*\.tar|.*\.exe)$`, "Default exclude pattern. Set it to empty string if you need to")
	skipBinary := optFlag.BoolP("skipbinary", "y", false, "Skip binary file")
	debug := optFlag.Bool("debug", false, "Enable debugging")
	expected_change_count := optFlag.Int("expected", -1, `Expected change count per 1 file. Apply to blockinfile command.
Default is -1 means do not care. Otherwise the program will panic if the number of change > expected_change_count.
It will automatically turn on backup`)

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

	switch file_path {
	case "version":
		printVersionBuildInfo()
		os.Exit(0)
	case "-":
		stdinContent := u.Must(io.ReadAll(os.Stdin))
		*grep = u.Ternary(*grep == "", *regexptn, *grep)
		ls, _ := u.Grep(string(stdinContent), *grep, true, false)
		fmt.Fprint(os.Stdout, strings.Join(ls, "\n"))
		return
	}

	if *grep != "" {
		*state = "print"
		*regexptn = *grep
		*search_string = ""
		*backup = false
		*line = ""
	}

	if *expected_change_count != -1 {
		*backup = true
	}

	opt := u.NewLineInfileOpt(&u.LineInfileOpt{
		Insertafter:   *insertafter,
		Insertbefore:  *insertbefore,
		Line:          *line,
		Regexp:        *regexptn,
		Search_string: *search_string,
		State:         *state,
		Backup:        *backup,
	})
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

	if u.FileExistsV2(file_path) != nil {
		u.FileTouch(file_path)
	}

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
				err, changed := u.LineInFile(path, opt)
				u.CheckErrNonFatal(err, "main lineinfile")
				output[path] = []interface{}{changed, err}
				if !isthereChange && changed {
					isthereChange = true
				}
			case "search_replace", "replace":
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
				// println("[DEBUG] upperBound " + *insertafter)
				// println("[DEBUG] lowerBound " + *insertbefore)
				u.CheckErr(json.Unmarshal([]byte(*insertafter), &upperBound), "Unmarshal upperBound "+*insertafter)
				u.CheckErr(json.Unmarshal([]byte(*insertbefore), &lowerBound), "Unmarshal lowerBound "+*insertbefore)
				u.CheckErr(json.Unmarshal([]byte(*regexptn), &marker), "Unmarshal marker "+*regexptn)

				if *state == "extract" {
					block, start_no, end_no, start_line := "", 0, 0, 0
					output[path] = []any{}
					for {
						matchedPattern := [][]string{}
						block, start_no, end_no, _, matchedPattern = u.ExtractTextBlockContains(path, upperBound, lowerBound, marker, start_line)
						if block == "" {
							break
						}
						resLines := strings.Split(block, "\n")
						content_no_boundary := strings.Join(resLines[len(upperBound):(len(resLines)-len(lowerBound))+1], "\n")
						output[path] = append(output[path], map[string]any{"content": block, "content_no_boundary": content_no_boundary, "start_line_no": start_no, "end_line_no": end_no, "matched": matchedPattern})
						start_line = end_no
					}
					return nil
				}
				oldblock, start, end, start_line := "", 0, 0, 0
				output[path] = []any{}
				changed_count := 0
				if *backup {
					u.CheckErr(u.Copy(path, path+".bak"), "Backup "+path)
				}
				insertIfNotFound := true
				for {
					if start_line > 0 { // If we did once then we dont insert anymore for the rest of text
						insertIfNotFound = false
					}
					oldblock, start, end, _ = u.BlockInFile(path, upperBound, lowerBound, marker, *line, *state == "keepboundary", false, start_line, map[string]any{"insertIfNotFound": insertIfNotFound})
					if oldblock == "" {
						break
					}
					output[path] = append(output[path], map[string]any{"changed": true, "start_line_no": start, "end_line_no": end})
					if !isthereChange {
						isthereChange = true
					}
					start_line = end
					changed_count++
				}
				fmt.Fprintf(os.Stderr, "file: %s - changes: %d - expected changes: %d\n", path, changed_count, *expected_change_count)
				if *expected_change_count > 0 && changed_count != *expected_change_count {
					panic(fmt.Sprintf("[ERROR] File '%s' | changed_count %d not match with expected_change_count %d\n", path, changed_count, *expected_change_count))
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
		fmt.Println(u.JsonDump(output, ""))
		if *erroIfNoChanged && !isthereChange {
			fmt.Fprintf(os.Stderr, "[ERROR] expected changed but no changed\n")
			os.Exit(1)
		}
	}
}
