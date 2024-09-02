package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
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

// Output format of each line. A file may have many lines; each line may have more than 1 creds pair matches
type OutputFmt struct {
	File    string
	Line_no int
	Pattern string
	Matches []string
}

// The output format of the program
// map of filename => map of line_no => OutputFmt
// Design like this so we can lookup by file name and line number quickly using hash map (O1 lookup) to compare between runs
type ProjectOutputFmt map[string]map[int]OutputFmt

// loadProfile to load a existing previous run output into map and used it to compare this run against.
func loadProfile(filename string) (output ProjectOutputFmt, err error) {
	datab, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(datab, &output)
	return output, err
}

// cred_detect_ProcessFiles to process a batch of files to detect credential pattern and send result to output_chan
func cred_detect_ProcessFiles(wg *sync.WaitGroup, fileBatch map[string]fs.FileInfo, cred_ptn_compiled map[string]*regexp.Regexp, password_check_mode, words_file_path string, entropy_threshold float64, output_chan chan<- OutputFmt, log_chan chan<- string, debug bool) {
	defer wg.Done()

	load_profile_path := os.Getenv("LOAD_PROFILE_PATH")
	previous_run_result := ProjectOutputFmt{}

	if load_profile_path != "" {
		var err error
		previous_run_result, err = loadProfile(load_profile_path)
		u.CheckErrNonFatal(err, "[WARN] can not load profile "+load_profile_path)
	}

	for fpath, finfo := range fileBatch {
		datab, err := os.ReadFile(fpath)
		if err1 := u.CheckErrNonFatal(err, "ReadFile "+fpath); err1 != nil {
			log_chan <- err1.Error()
			continue
		}
		datalines := strings.Split(string(datab), "\n")
		if strings.HasSuffix(path.Ext(finfo.Name()), "js") && len(datalines) < 10 && finfo.Size() >= 1000 { // Skip as it is likely js minified file
			continue
		}
		for idx, data := range datalines {
			for ptnStr, ptn := range cred_ptn_compiled {
				matches := ptn.FindAllStringSubmatch(data, -1)
				if len(matches) > 0 {
					o := OutputFmt{
						File:    fpath,
						Line_no: idx,
						Pattern: ptnStr,
						Matches: []string{},
					}
					var oldmatches, newmatches mapset.Set[string]
					if check_prev, ok := previous_run_result[fpath]; ok {
						if _, ok := check_prev[idx]; ok {
							oldmatches = mapset.NewSet[string](previous_run_result[fpath][idx].Matches...)
						}
					}
					for _, match := range matches {
						if debug {
							log_chan <- fmt.Sprintf("%s:%d - %s: %s", fpath, idx, match[1], match[2])
						}

						if len(match) > 1 && ag.IsLikelyPasswordOrToken(match[2], password_check_mode, words_file_path, 4, entropy_threshold) {
							if debug || load_profile_path != "" {
								o.Matches = append(o.Matches, match[1], match[2])
							} else {
								o.Matches = append(o.Matches, match[1], "*****")
							}
						}
					}
					if len(o.Matches) > 0 {
						newmatches = mapset.NewSet[string](o.Matches...)
						if load_profile_path != "" && oldmatches != nil && newmatches != nil && oldmatches.Equal(newmatches) {
							log_chan <- fmt.Sprintf("File: %s - Line: %d matches exist in profile, skipping", fpath, idx)
						} else {
							if load_profile_path != "" {
								for idx, _ := range o.Matches {
									if idx%2 == 1 {
										o.Matches[idx] = "*****"
									}
								}
							}
							output_chan <- o
						}
					}
				}
			}
		}
	}
}

func main() {
	optFlag := pflag.NewFlagSet("opt", pflag.ExitOnError)
	// config_file := optFlag.String("project-config", "", "File Path to Exclude pattern")
	cred_regexptn := optFlag.StringArrayP("regexp", "r", []string{}, "List pattern to detect credential values")
	default_cred_regexptn := optFlag.StringArrayP("default-regexp", "p", Credential_patterns, "Default list of credencial pattern.")
	filename_ptn := optFlag.StringP("fptn", "f", ".*", "Filename regex pattern")
	exclude := optFlag.StringP("exclude", "e", "", "Exclude file name pattern")
	path_exclude := optFlag.String("path-exclude", "", "File Path to Exclude pattern")
	load_profile_path := optFlag.String("profile", "", "File Path to load the result from previous run")
	defaultExclude := optFlag.StringP("defaultexclude", "d", `^(\.git|.*\.zip|.*\.gz|.*\.xz|.*\.bz2|.*\.zstd|.*\.7z|.*\.dll|.*\.iso|.*\.bin|.*\.tar|.*\.exe)$`, "Default exclude pattern. Set it to empty string if you need to")
	skipBinary := optFlag.BoolP("skipbinary", "y", true, "Skip binary file")
	password_check_mode := optFlag.String("check-mode", "letter+word", "Password check mode. List of allowed values: letter, digit, special, letter+digit, letter+digit+word, all. The default value (letter+digit+word) requires a file /tmp/words.txt; it will automatically download it if it does not exist. Link to download https://github.com/dwyl/english-words/blob/master/words.txt . It describes what it looks like a password for example if the value is 'letter' means any random ascii letter can be treated as password and will be reported. Same for others, eg, letter+digit+word means value has letter, digit and NOT looks like English word will be treated as password. Value 'all' is like letter+digit+special ")
	words_list_url := optFlag.String("words-list-url", "https://raw.githubusercontent.com/dwyl/english-words/master/words.txt", "Word list url to download")

	debug := optFlag.Bool("debug", false, "Enable debugging. Note that it will print password values unmasked. Do not run it on CI/CD")
	save_config_file := optFlag.String("save-config", "cred-detect-config.yaml", "Path to save config from command flags to a yaml file")

	file_path := os.Args[1]
	optFlag.Usage = func() {
		fmt.Printf(`Usage: %s [filename/path] [opt]
		Run with option -h for complete help.
		The app search for config file named 'cred-detect-config.yaml' in any of
		  - the current working directory,
		  - $HOME/.config
		  - /etc/cred-detect

		The command line options has higher priority. Config file existance is optional however you can save the current commandline
		opts into config file using option '--save-config'; by default it is enabled to save it to the current directory.

		***** WORKFLOW *****
		cd <project-to-scan-root-dir>
		./cred-detect . --debug <extra-opt> > cred-detect-profile.json
		# extra-opt if u need, mostly depending on each project you may optimize the exclude option or even change the regex pattern etc
		# examine the json file and see any false positive case; if they are, leave it in the profile. Fix up your code for real case.
		# Re-run the above until all data in json file are false positive.
		# commit the profile file and the cred-detect-config.yaml into your project git.
		# Now in CI/CD design the command to run like this

		cd <project>
		./cred-detect . --profile cred-detect-profile.json --debug=false

		It will discover new real case from now on.

		Options below:

		`, os.Args[0])
		optFlag.PrintDefaults()
	}
	optFlag.Parse(os.Args[1:])
	viper.BindPFlags(optFlag)

	viper.SetConfigName("cred-detect-config") // name of config file (without extension)
	viper.SetConfigType("yaml")               // REQUIRED if the config file does not have the extension in the name
	viper.AddConfigPath("/etc/cred-detect/")  // path to look for the config file in
	viper.AddConfigPath("$HOME/.config/")     // call multiple times to add many search paths
	viper.AddConfigPath(".")                  // optionally look for config in the working directory
	err := viper.ReadInConfig()               // Find and read the config file
	if err != nil {                           // Handle errors reading the config file
		fmt.Fprintf(os.Stderr, "[WARN] config file not found - %s\n", err.Error())
	}

	if *save_config_file != "" {
		viper.WriteConfigAs(*save_config_file)
	}

	*cred_regexptn = viper.GetStringSlice("regexp")
	*default_cred_regexptn = viper.GetStringSlice("default-regexp")
	*filename_ptn = viper.GetString("fptn")
	*exclude = viper.GetString("exclude")
	*path_exclude = viper.GetString("path-exclude")
	*load_profile_path = viper.GetString("profile")
	*defaultExclude = viper.GetString("defaultexclude")
	*skipBinary = viper.GetBool("skipbinary")
	*password_check_mode = viper.GetString("check-mode")
	*words_list_url = viper.GetString("words-list-url")
	*debug = viper.GetBool("debug")

	if len(*cred_regexptn) > 0 {
		*default_cred_regexptn = append(*default_cred_regexptn, *cred_regexptn...)
	}

	if strings.Contains(*password_check_mode, "word") {
		if res, _ := u.FileExists("/tmp/words.txt"); !res {
			fmt.Println("Downloading words.txt")
			u.Curl("GET", *words_list_url, "", "/tmp/words.txt", []string{})
		}
	}

	os.Setenv("LOAD_PROFILE_PATH", *load_profile_path)

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

	var path_exclude_ptn *regexp.Regexp = nil
	if *path_exclude != "" {
		path_exclude_ptn = regexp.MustCompile(*path_exclude)
	}

	output := ProjectOutputFmt{}
	logs := []string{}
	var wg sync.WaitGroup
	output_chan := make(chan OutputFmt)
	log_chan := make(chan string)
	// Setup the harvest worker
	go func(output *ProjectOutputFmt, logs *[]string, output_chan <-chan OutputFmt, log_chan <-chan string) {
		for {
			select {
			case msg, morelog := <-log_chan:
				*logs = append(*logs, msg)
				if !morelog {
					log_chan = nil
				}
			case out, moredata := <-output_chan:
				if out.File == "" {
					continue
				}
				val, ok := (*output)[out.File]
				if !ok {
					(*output)[out.File] = map[int]OutputFmt{}
					val = (*output)[out.File]
				}
				val[out.Line_no] = out

				if !moredata {
					output_chan = nil
				}
			}
			if log_chan == nil && output_chan == nil {
				// use like this might not be needed as after wg is done the main thread go ahead and print out thigns and then quit, this go routine will be gone too
				// however it looks better to close channel in main thread; detect and then break here
				fmt.Fprintln(os.Stderr, "Channels closed, quit harvestor")
				break
			}
		}
	}(&output, &logs, output_chan, log_chan)
	// 10 is fastest
	batchSize := 10
	filesBatch := map[string]fs.FileInfo{}

	err1 := filepath.Walk(file_path, func(fpath string, info fs.FileInfo, err error) error {
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			return nil
		}
		if path_exclude_ptn != nil {
			if path_exclude_ptn.MatchString(fpath) {
				fmt.Fprintf(os.Stderr, "SKIP PATH %s\n", fpath)
				return nil
			}
		}
		fname := info.Name()
		if info.IsDir() && ((excludePtn != nil && excludePtn.MatchString(fname)) || (defaultExcludePtn != nil && defaultExcludePtn.MatchString(fname))) {
			fmt.Fprintf(os.Stderr, "SKIP DIR %s\n", fpath)
			return filepath.SkipDir
		}
		// Check if the file matches the pattern

		if !info.IsDir() && filename_regexp.MatchString(fname) && ((excludePtn == nil) || (excludePtn != nil && !excludePtn.MatchString(fname))) && ((defaultExcludePtn == nil) || (defaultExcludePtn != nil && !defaultExcludePtn.MatchString(fname))) {
			if *skipBinary {
				isbin, err := ag.IsBinaryFileSimple(fpath)
				if (err == nil) && isbin {
					fmt.Fprintf(os.Stderr, "SKIP BIN %s\n", fpath)
					return nil
				}
			}

			fmode := info.Mode()
			if !(fmode.IsRegular()) {
				return nil
			}
			if len(filesBatch) < batchSize {
				filesBatch[fpath] = info
			} else {
				wg.Add(1)
				go cred_detect_ProcessFiles(&wg, filesBatch, cred_ptn_compiled, *password_check_mode, "/tmp/words.txt", 0, output_chan, log_chan, *debug)
				filesBatch = map[string]fs.FileInfo{}
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
		fmt.Fprintln(os.Stderr, strings.Join(logs, "\n"))
	}
	if len(output) > 0 {
		// fmt.Printf("%s\n", u.JsonDump(output, "     "))
		je := json.NewEncoder(os.Stdout)
		je.SetEscapeHTML(false) // prevent < or > to be backspace like \uXXXX
		je.SetIndent("", "  ")
		je.Encode(output)
		os.Exit(1)
	} else {
		fmt.Print("{}")
	}
}
