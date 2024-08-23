package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/spf13/pflag"
	ag "github.com/sunshine69/automation-go/lib"
	u "github.com/sunshine69/golang-tools/utils"
)

// var Credential_patterns = []string{
// 	`(?i)['"]?password['"]?\s*[=:]\s*['"]?([^'"\s]+)['"]?`, // Matches "password [=:] value"
// 	`(?i)['"]?token['"]?\s*[=:]\s*['"]?([^'"\s]+)['"]?`,    // Matches "token [=:] value"
// 	`(?i)['"]?api_key['"]?\s*[=:]\s*['"]?([^'"\s]+)['"]?`,  // Matches "api_key [=:] value"
// 	`(?i)['"]?secret['"]?\s*[=:]\s*['"]?([^'"\s]+)['"]?`,   // Matches "secret [=:] value"
// }

var Credential_patterns = []string{
	`(?i)['"]?(password|passwd|token|api_key|secret)['"]?[=:\s][\s]*?['"]?([^'"\s]+)['"]?`,
}

type OutputFmt struct {
	File    string
	Line_no int
	Pattern string
	Matches []string
}

func cred_detect_ProcessFiles(wg *sync.WaitGroup, fileBatch []string, cred_ptn_compiled map[string]*regexp.Regexp, password_check_mode, words_file_path string, entropy_threshold float64, output_chan chan<- OutputFmt, log_chan chan<- string, debug bool) {
	defer wg.Done()

	newline_byte := []byte("\n")
	for _, path := range fileBatch {
		datab, err := os.ReadFile(path)
		if err1 := u.CheckErrNonFatal(err, "ReadFile "+path); err1 != nil {
			log_chan <- err1.Error()
		}
		datalines := bytes.Split(datab, newline_byte)
		for idx, data := range datalines {
			for ptnStr, ptn := range cred_ptn_compiled {
				matches := ptn.FindAllSubmatch(data, -1)
				if len(matches) > 1 {
					o := OutputFmt{
						File:    path,
						Line_no: idx,
						Pattern: ptnStr,
						Matches: []string{},
					}
					for _, match := range matches {
						if debug {
							log_chan <- fmt.Sprintf("%s:%d - %s: %s\n", path, idx, string(match[1]), string(match[2]))
						}
						passVal := string(match[2])
						if len(match) > 1 && ag.IsLikelyPasswordOrToken(passVal, password_check_mode, words_file_path, entropy_threshold) {
							if debug {
								o.Matches = append(o.Matches, string(match[1]), string(match[2]))
							} else {
								o.Matches = append(o.Matches, string(match[1]), "*****")
							}
						}
					}
					if len(o.Matches) > 0 && o.File != "" {
						output_chan <- o
					}
				}
			}
		}
	}
}

func main() {
	optFlag := pflag.NewFlagSet("opt", pflag.ExitOnError)
	cred_regexptn := optFlag.StringArrayP("regexp", "r", []string{}, "List pattern to detect credential values")
	default_cred_regexptn := optFlag.StringArrayP("default-regexp", "p", Credential_patterns, "Default list of crec pattern.")
	filename_ptn := optFlag.StringP("fptn", "f", ".*", "Filename regex pattern")
	exclude := optFlag.StringP("exclude", "e", "", "Exclude file name pattern")
	defaultExclude := optFlag.StringP("defaultexclude", "d", `^(\.git|.*\.zip|.*\.gz|.*\.xz|.*\.bz2|.*\.zstd|.*\.7z|.*\.dll|.*\.iso|.*\.bin|.*\.tar|.*\.exe)$`, "Default exclude pattern. Set it to empty string if you need to")
	skipBinary := optFlag.BoolP("skipbinary", "y", false, "Skip binary file")
	password_check_mode := optFlag.String("check-mode", "letter+digit+word", "Password check mode. List of allowed values: letter, digit, special, letter+digit, letter+digit+word, all. The default value (letter+digit+word) requires a file /tmp/words.txt; it will automatically download it if it does not exist. Link to download https://github.com/dwyl/english-words/blob/master/words.txt . It describes what it looks like a password for example if the value is 'letter' means any random ascii letter can be treated as password and will be reported. Same for others, eg, letter+digit+word means value has letter, digit and NOT looks like English word will be treated as password. Value 'all' is like letter+digit+special ")
	words_list_url := optFlag.String("words-list-url", "https://raw.githubusercontent.com/dwyl/english-words/master/words.txt", "Word list url to download")

	debug := optFlag.Bool("debug", false, "Enable debugging")

	file_path := os.Args[1]
	optFlag.Usage = func() {
		fmt.Printf("Usage: %s [filename/path] [opt]\n", os.Args[0])
		optFlag.PrintDefaults()
	}
	optFlag.Parse(os.Args[1:])

	if len(*cred_regexptn) > 0 {
		*default_cred_regexptn = append(*default_cred_regexptn, *cred_regexptn...)
	}

	if strings.Contains(*password_check_mode, "word") {
		if res, _ := u.FileExists("/tmp/words.txt"); !res {
			fmt.Println("Downloading words.txt")
			u.Curl("GET", *words_list_url, "", "/tmp/words.txt", []string{})
		}
	}

	cred_ptn_compiled := map[string]*regexp.Regexp{}
	for _, ptn := range *default_cred_regexptn {
		cred_ptn_compiled[ptn] = regexp.MustCompile(ptn)
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

	output := map[string]OutputFmt{}
	logs := []string{}
	var wg sync.WaitGroup
	output_chan := make(chan OutputFmt)
	log_chan := make(chan string)
	// Setup the harvest worker
	go func(output *map[string]OutputFmt, logs *[]string, output_chan <-chan OutputFmt, log_chan <-chan string) {
		var morelog, moredata bool
		var msg string
		var out OutputFmt
		for {
			select {
			case msg, morelog = <-log_chan:
				*logs = append(*logs, msg)
			case out, moredata = <-output_chan:
				if out.File != "" {
					(*output)[out.File] = out
				}
			default:
				if !morelog && !moredata {
					break
				}
			}
		}
	}(&output, &logs, output_chan, log_chan)
	// 10 is fastest
	batchSize := 10
	filesBatch := []string{}

	err1 := filepath.Walk(file_path, func(path string, info fs.FileInfo, err error) error {
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
				isbin, err := ag.IsBinaryFileSimple(path)
				if (err == nil) && isbin {
					return nil
				}
			}

			finfo, err := os.Stat(path)
			if err1 := u.CheckErrNonFatal(err, "LineInFile Stat"); err1 != nil {
				return nil
			}
			fmode := finfo.Mode()
			if !(fmode.IsRegular()) {
				return nil
			}
			if len(filesBatch) < batchSize {
				filesBatch = append(filesBatch, path)
			} else {
				wg.Add(1)
				go cred_detect_ProcessFiles(&wg, filesBatch, cred_ptn_compiled, *password_check_mode, "/tmp/words.txt", 0, output_chan, log_chan, *debug)
				filesBatch = []string{}
			}
		}
		return nil
	})

	if len(filesBatch) > 0 { // Last batch
		wg.Add(1)
		go cred_detect_ProcessFiles(&wg, filesBatch, cred_ptn_compiled, *password_check_mode, "/tmp/words.txt", 0, output_chan, log_chan, *debug)
	}

	wg.Wait()
	close(log_chan)
	close(output_chan)

	if err1 != nil {
		panic(err1.Error())
	}
	if len(logs) > 0 {
		fmt.Println(strings.Join(logs, "\n"))
	}
	if len(output) > 0 {
		fmt.Printf("%s\n", u.JsonDump(output, "     "))
		os.Exit(1)
	}
}
