package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

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
	`(?i)['"]?(password|passwd|token|api_key|secret)['"]?[=:]?[\s]*?['"]?([^'"\s]+)['"]?`,
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

	// debug := optFlag.Bool("debug", false, "Enable debugging")

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
	type OutputFmt struct {
		Line_no int
		Pattern string
		Matches []string
	}
	output := map[string]OutputFmt{}
	newline_byte := []byte("\n")

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

			datab, err := os.ReadFile(path)
			if err1 := u.CheckErrNonFatal(err, "ReadFile "+path); err1 != nil {
				return nil
			}
			datalines := bytes.Split(datab, newline_byte)
			for idx, data := range datalines {
				for ptnStr, ptn := range cred_ptn_compiled {
					matches := ptn.FindAllSubmatch(data, -1)
					if len(matches) > 1 {
						o := OutputFmt{
							Line_no: idx,
							Pattern: ptnStr,
							Matches: []string{},
						}
						for _, match := range matches {
							fmt.Printf("%s - %s\n", string(match[1]), string(match[2]))
							passVal := string(match[2])
							if len(match) > 1 && ag.IsLikelyPasswordOrToken(passVal, *password_check_mode, "/tmp/words.txt") {
								o.Matches = append(o.Matches, string(match[1]), ag.MaskCredentialByte(match[2]))
							}
						}
						if len(o.Matches) > 0 {
							output[path] = o
						}
					}
				}
			}
		}
		return nil
	})
	if err1 != nil {
		panic(err1.Error())
	}
	fmt.Printf("%s\n", u.JsonDump(output, "     "))
}
