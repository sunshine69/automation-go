package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"

	"github.com/spf13/pflag"
	ag "github.com/sunshine69/automation-go/lib"
	u "github.com/sunshine69/golang-tools/utils"
)

var Credential_patterns = []string{
	`(?i)['"]?password['"]?\s*[=:]\s*['"]?([^'"\s]+)['"]?`, // Matches "password [=:] value"
	`(?i)['"]?token['"]?\s*[=:]\s*['"]?([^'"\s]+)['"]?`,    // Matches "token [=:] value"
	`(?i)['"]?api_key['"]?\s*[=:]\s*['"]?([^'"\s]+)['"]?`,  // Matches "api_key [=:] value"
	`(?i)['"]?secret['"]?\s*[=:]\s*['"]?([^'"\s]+)['"]?`,   // Matches "secret [=:] value"
}

func main() {
	optFlag := pflag.NewFlagSet("opt", pflag.ExitOnError)
	cred_regexptn := optFlag.StringArrayP("regexp", "r", []string{}, "List pattern to detect credential values")
	default_cred_regexptn := optFlag.StringArrayP("default-regexp", "p", Credential_patterns, "Default list of crec pattern.")
	filename_ptn := optFlag.StringP("fptn", "f", ".*", "Filename regex pattern")
	exclude := optFlag.StringP("exclude", "e", "", "Exclude file name pattern")
	defaultExclude := optFlag.StringP("defaultexclude", "d", `^(\.git|.*\.zip|.*\.gz|.*\.xz|.*\.bz2|.*\.zstd|.*\.7z|.*\.dll|.*\.iso|.*\.bin|.*\.tar|.*\.exe)$`, "Default exclude pattern. Set it to empty string if you need to")
	skipBinary := optFlag.BoolP("skipbinary", "y", false, "Skip binary file")
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
	cred_ptn_compiled := []*regexp.Regexp{}
	for _, ptn := range *default_cred_regexptn {
		cred_ptn_compiled = append(cred_ptn_compiled, regexp.MustCompile(ptn))
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
	output := map[string][]string{}

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
			datab, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			// if err1 := u.CheckErrNonFatal(err, "ReadFile "+path); err1 != nil {
			// 	return nil
			// }
			data := string(datab)
			for _, ptn := range cred_ptn_compiled {
				matches := ptn.FindAllStringSubmatch(data, -1)
				if len(matches) > 1 {
					for _, match := range matches {
						if len(match) > 1 && ag.IsLikelyPasswordOrToken(match[1], "") {
							output[path] = []string{}
							output[path] = append(output[path], match[0], match[1])
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
