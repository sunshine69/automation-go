package main

import (
	"encoding/json"
	"fmt"

	"io/fs"
	"os"
	"path"
	"path/filepath"

	// regexp "github.com/wasilibs/go-re2"

	"regexp"
	"strings"
	"sync"

	"github.com/sunshine69/automation-go/lib"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	u "github.com/sunshine69/golang-tools/utils"
)

//	var Credential_patterns = []string{
//		`(?i)['"]?password['"]?\s*[=:]\s*['"]?([^'"\s]+)['"]?`, // Matches "password [=:] value"
//		`(?i)['"]?token['"]?\s*[=:]\s*['"]?([^'"\s]+)['"]?`,    // Matches "token [=:] value"
//		`(?i)['"]?api_key['"]?\s*[=:]\s*['"]?([^'"\s]+)['"]?`,  // Matches "api_key [=:] value"
//		`(?i)['"]?secret['"]?\s*[=:]\s*['"]?([^'"\s]+)['"]?`,   // Matches "secret [=:] value"
//	}
//
// https://github.com/l4yton/RegHex?tab=readme-ov-file#artifactory-api-token
// https://github.com/neospl0it/regexscan/blob/main/regexscan.sh
var (
	Credential_patterns = []string{
		`(?i)['"]?(password|passwd|token|api_key|secret|access_key|admin_pass|algolia_admin_key|algolia_api_key|alias_pass|aos_key|api_key_sid|apikey|apiSecret|app_debug|app_id|app_key|appkey|appkeysecret|application_key|appsecret|appspot|aws_access|aws_access_key_id|aws_key|aws_secret|aws_secret_key|aws_token|AWSSecretKey|b2_app_key|bintray_apikey|bintray_gpg_password|bintray_key|bintraykey|bluemix_api_key|bluemix_pass|browserstack_access_key|bucket_password|bucketeer_aws_access_key_id|bucketeer_aws_secret_access_key|built_branch_deploy_key|bx_password|cache_s3_secret_key|cattle_access_key|cattle_secret_key|certificate_password|ci_deploy_password|client_secret|client_zpk_secret_key|clojars_password|cloud_api_key|cloud_watch_aws_access_key|cloudant_password|cloudflare_api_key|cloudflare_auth_key|cloudinary_api_secret|cloudinary_name|codecov_token|connectionstring|consumer_key|consumer_secret|credentials|cypress_record_key|database_password|database_schema_test|datadog_api_key|datadog_app_key|db_password|db_server|db_username|dbpasswd|dbpassword|deploy_password|digitalocean_ssh_key_body|digitalocean_ssh_key_ids|docker_hub_password|docker_key|docker_pass|docker_passwd|docker_password|dockerhub_password|dockerhubpassword|droplet_travis_password|dynamoaccesskeyid|dynamosecretaccesskey|elastica_host|elastica_port|elasticsearch_password|encryption_key|encryption_password|env.heroku_api_key|env.sonatype_password|eureka.awssecretkey)['"]?[\s]?[=:\s\|][\s]*?['"]?([^'"\s]+)['"]?`,
		// `(?i)((access_key|access_token|admin_pass|admin_user|algolia_admin_key|algolia_api_key|alias_pass|alicloud_access_key|amazon_secret_access_key|amazonaws|ansible_vault_password|aos_key|api_key|api_key_secret|api_key_sid|api_secret|api.googlemaps AIza|apidocs|apikey|apiSecret|app_debug|app_id|app_key|app_log_level|app_secret|appkey|appkeysecret|application_key|appsecret|appspot|auth_token|authorizationToken|authsecret|aws_access|aws_access_key_id|aws_bucket|aws_key|aws_secret|aws_secret_key|aws_token|AWSSecretKey|b2_app_key|bashrc password|bintray_apikey|bintray_gpg_password|bintray_key|bintraykey|bluemix_api_key|bluemix_pass|browserstack_access_key|bucket_password|bucketeer_aws_access_key_id|bucketeer_aws_secret_access_key|built_branch_deploy_key|bx_password|cache_driver|cache_s3_secret_key|cattle_access_key|cattle_secret_key|certificate_password|ci_deploy_password|client_secret|client_zpk_secret_key|clojars_password|cloud_api_key|cloud_watch_aws_access_key|cloudant_password|cloudflare_api_key|cloudflare_auth_key|cloudinary_api_secret|cloudinary_name|codecov_token|config|conn.login|connectionstring|consumer_key|consumer_secret|credentials|cypress_record_key|database_password|database_schema_test|datadog_api_key|datadog_app_key|db_password|db_server|db_username|dbpasswd|dbpassword|dbuser|deploy_password|digitalocean_ssh_key_body|digitalocean_ssh_key_ids|docker_hub_password|docker_key|docker_pass|docker_passwd|docker_password|dockerhub_password|dockerhubpassword|dot-files|dotfiles|droplet_travis_password|dynamoaccesskeyid|dynamosecretaccesskey|elastica_host|elastica_port|elasticsearch_password|encryption_key|encryption_password|env.heroku_api_key|env.sonatype_password|eureka.awssecretkey)[a-z0-9_ .\-,]{0,25})(=|>|:=|\|\|:|<=|=>|:).{0,5}['"]([0-9a-zA-Z\-_=]{8,64})['"]`,
	}
	version   string // Will hold the version number
	buildTime string // Will hold the build time
	// In the regex above, which group index to refer the password part (which we call a) and the content of secret (b)
	// This allows us to extract the data in the report when matching agains the regex. The default value matched with the default ptn
	group_index [][]int = [][]int{{1, 2}}
	// group_index [][]int = [][]int{{2, 3}}
	WordDict map[string]struct{} = nil
)

// Output format of each line. A file may have many lines; each line may have more than 1 creds pair matches
type OutputFmt struct {
	File    string
	Line_no []int
	Pattern string
	Matches []string
}

// The output format of the program
// map of filename => map of TokenName+TokenValue => OutputFmt
// Design like this so we can lookup by file name and line number quickly using hash map (O1 lookup) to compare between runs
type ProjectOutputFmt map[string]map[string]OutputFmt

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
func cred_detect_ProcessFiles(wg *sync.WaitGroup, fileBatch map[string]fs.FileInfo, password_check_mode string, entropy_threshold float64, output_chan chan<- OutputFmt, log_chan chan<- string, debug bool) {
	defer wg.Done()
	load_profile_path := os.Getenv("LOAD_PROFILE_PATH")
	previous_run_result := ProjectOutputFmt{}

	if load_profile_path != "" {
		var err error
		previous_run_result, err = loadProfile(load_profile_path)
		if u.CheckErrNonFatal(err, "[WARN] can not load profile "+load_profile_path) != nil {
			os.Setenv("LOAD_PROFILE_PATH", "")
		}
	}
	compiledPtn := map[string]*regexp.Regexp{}
	for _, ptn := range Credential_patterns {
		compiledPtn[ptn] = regexp.MustCompile(ptn)
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
		o := OutputFmt{
			File:    fpath,
			Line_no: []int{},
			Matches: []string{},
		}

		for idx, data := range datalines {
			ptn_idx := 0
			for ptn, ptnCom := range compiledPtn {
				matches := ptnCom.FindAllStringSubmatch(data, -1)
				if len(matches) > 0 {
					o.Pattern = ptn
					o.Line_no = append(o.Line_no, idx)

					var oldmatches map[string]OutputFmt
					if check_prev, ok := previous_run_result[fpath]; ok {
						oldmatches = check_prev
					}
					for _, match := range matches {
						// fmt.Printf("[DEBUG] %s\n", u.JsonDump(match, ""))
						if debug {
							log_chan <- fmt.Sprintf("%s:%d - %s: %s", fpath, idx, match[group_index[ptn_idx][0]], match[group_index[ptn_idx][1]])
						}

						if len(match) > 1 && lib.IsLikelyPasswordOrToken(match[group_index[ptn_idx][1]], password_check_mode, WordDict, 4, entropy_threshold) {
							o.Matches = append(o.Matches, match[group_index[ptn_idx][0]], match[group_index[ptn_idx][1]])
						}
					}
					if len(o.Matches) > 0 {
						match_Sig := o.Matches[0] + o.Matches[1]
						if _, ok := oldmatches[match_Sig]; ok {
							if debug {
								log_chan <- fmt.Sprintf("File: %s - matches %s exist in profile, skipping", fpath, match_Sig)
							}
						} else {
							if !debug { // Mask value
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
				ptn_idx++
			}
		}
	}
}

func printVersionBuildInfo() {
	fmt.Printf("Version: %s\nBuild time: %s\n", version, buildTime)
}

func main() {
	optFlag := pflag.NewFlagSet("opt", pflag.ExitOnError)
	// config_file := optFlag.String("project-config", "", "File Path to Exclude pattern")
	batchSize := optFlag.Int("batch-size", 8, "Batch size - the number of files in a batch to process.")
	cred_regexptn := optFlag.StringArrayP("regexp", "r", []string{}, "List pattern to detect credential values. The app has a default pattern already. This allows you to add your own pattern as well if required. If you add your own pattern it will use it as extra pattern, that is the default one built-in code still used. Remember to use option -a and -b to set the group index pair for your pattern. Example: if you add two more patterns, and the first one you have set the capture group for key pass is 1 and for value of credential is 3; the second pattern is 2 and 4 then you should pass option -a=1,2 -b=3,4")
	pattern_group_index_ap := optFlag.IntSliceP("group-index-a", "a", []int{}, "Set the group index pair to capture. If you customize the regexp pattern then supply this to hint which group index is used for the first capture part (the leading string to identify what comes next is the credential). The order is map 1<->1 that is first regex map with first item in here. Value format is a coma separated string such as -a=1,2,3")
	pattern_group_index_bp := optFlag.IntSliceP("group-index-b", "b", []int{}, "Similar to --group-index-a. Set the group index to capture the credential itself.")
	filename_ptn := optFlag.StringP("fptn", "f", ".*", "Filename regex pattern")
	exclude := optFlag.StringP("exclude", "e", "", "Exclude file name pattern")
	path_exclude := optFlag.StringSlice("path-exclude", []string{""}, "File Path to Exclude pattern")
	load_profile_path := optFlag.String("profile", "", "File Path to load the result from previous run")
	defaultExclude := optFlag.StringP("defaultexclude", "d", `^(\.git|.*\.zip|.*\.gz|.*\.xz|.*\.bz2|.*\.zstd|.*\.7z|.*\.dll|.*\.iso|.*\.bin|.*\.tar|.*\.exe)$`, "Default exclude pattern. Set it to empty string if you need to")
	skipBinary := optFlag.BoolP("skipbinary", "y", true, "Skip binary file")
	password_check_mode := optFlag.String("check-mode", "letter+word", "Password check mode. List of allowed values: letter, digit, special, letter+digit, letter+digit+word, all. The default value (letter+digit+word) requires a file /tmp/words.txt; it will automatically download it if it does not exist. Link to download https://github.com/dwyl/english-words/blob/master/words.txt . It describes what it looks like a password for example if the value is 'letter' means any random ascii letter can be treated as password and will be reported. Same for others, eg, letter+digit+word means value has letter, digit and NOT looks like English word will be treated as password. Value 'all' is like letter+digit+special ")
	words_list_url := optFlag.String("words-list-url", "https://github.com/dwyl/english-words/blob/master/words.txt", "Word list url to download")
	words_dir_path := optFlag.String("words-dir", "", "Words directory path to search for or download. Default is system temp dir")

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
		cred-detect . --debug <extra-opt> --profile="" > cred-detect-profile.json
		# extra-opt if u need, mostly depending on each project you may optimize the exclude option or even change the regex pattern etc
		# examine the json file and see any false positive case; if they are, leave it in the profile. Fix up your code for real case.
		# Re-run the above until all data in json file are false positive.
		# commit the profile file and the cred-detect-config.yaml into your project git.
		# Now in CI/CD design the command to run like this

		cd <project>
		cred-detect . --profile cred-detect-profile.json --debug=false

		It will discover new real case from now on. You can edit the profile json file to remove/add new ignore case.

		If you need to re-generate the profile then you need to delete the current profile file

		rm -f cred-detect-profile.json
		cred-detect . --debug=true > cred-detect-profile.json

		Also as the config file has already generated; you should have a look at the option in there to be sure the run is correct.

		To add the command to scan before you commit using git commit hook do the following

		Options below:

		`, os.Args[0])
		optFlag.PrintDefaults()
	}
	optFlag.Parse(os.Args[1:])

	if file_path == "version" {
		printVersionBuildInfo()
		os.Exit(0)
	}

	if *words_dir_path == "" {
		*words_dir_path = os.TempDir()
	}

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
	*filename_ptn = viper.GetString("fptn")
	*exclude = viper.GetString("exclude")
	*path_exclude = viper.GetStringSlice("path-exclude")
	*load_profile_path = viper.GetString("profile")
	*defaultExclude = viper.GetString("defaultexclude")
	*skipBinary = viper.GetBool("skipbinary")
	*password_check_mode = viper.GetString("check-mode")
	*words_list_url = viper.GetString("words-list-url")
	*debug = viper.GetBool("debug")
	word_file_path := path.Join(*words_dir_path, "cred-detect-word.txt")

	if len(*cred_regexptn) > 0 {
		Credential_patterns = append(Credential_patterns, *cred_regexptn...)
	}
	if len(*pattern_group_index_ap) > 0 && len(*pattern_group_index_bp) > 0 && len(*pattern_group_index_ap) == len(*pattern_group_index_bp) {
		for idx, v := range *pattern_group_index_ap {
			_b := *pattern_group_index_bp
			_tmp := []int{v, _b[idx]}
			group_index = append(group_index, _tmp)
		}
	}

	if strings.Contains(*password_check_mode, "word") {
		res, err := u.FileExists(word_file_path)
		if !res || err != nil {
			fmt.Fprintln(os.Stderr, "Downloading words.txt")
			u.Must(u.Curl("GET", *words_list_url, "", word_file_path, []string{}, nil))
		}
		fmt.Fprintf(os.Stderr, "[DEBUG] %v - %v\n", res, err)
		WordDict = u.Must(lib.LoadWordDictionary(word_file_path, 4))
	}

	os.Setenv("LOAD_PROFILE_PATH", *load_profile_path)

	filename_regexp := regexp.MustCompile(*filename_ptn)

	excludePtn := regexp.MustCompile(*exclude)
	if *exclude == "" {
		excludePtn = nil
	}

	defaultExcludePtn := regexp.MustCompile(*defaultExclude)
	if *defaultExclude == "" {
		defaultExcludePtn = nil
	}
	path_exclude_ptn := []*regexp.Regexp{}
	for _, ptn := range *path_exclude {
		path_exclude_ptn = append(path_exclude_ptn, regexp.MustCompile(ptn))
	}

	output := ProjectOutputFmt{}
	logs := []string{}
	var wg sync.WaitGroup
	output_chan := make(chan OutputFmt)
	log_chan := make(chan string)
	stat_chan := make(chan int)

	total_files_scanned, total_files_process := 0, 0
	// Setup the harvest worker
	harvestFunc := func(output *ProjectOutputFmt, logs *[]string, output_chan <-chan OutputFmt, log_chan <-chan string, stat_chan <-chan int) {
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

				tokenSig := out.Matches[0] + out.Matches[1]
				val, ok := (*output)[out.File] // Check if we already have this file
				if !ok {                       // If not we create new
					(*output)[out.File] = map[string]OutputFmt{}
					(*output)[out.File][tokenSig] = out
				} else { //If exist just add new tokenSig in
					val[tokenSig] = out
				}

				if !moredata {
					output_chan = nil
				}
			case file_count, more_file := <-stat_chan:
				total_files_process += file_count
				if !more_file {
					stat_chan = nil
				}
			}
			if log_chan == nil && output_chan == nil && stat_chan == nil {
				// use like this might not be needed as after wg is done the main thread go ahead and print out thigns and then quit, this go routine will be gone too
				// however it looks better to close channel in main thread; detect and then break here
				fmt.Fprintln(os.Stderr, "Channels closed, quit harvestor")
				break
			}
		}
	}
	go harvestFunc(&output, &logs, output_chan, log_chan, stat_chan)
	// go harvestFunc(&output2, &logs, output_chan, log_chan, stat_chan)
	// 10 is fastest. Spawn more harvest does not help
	filesBatch := map[string]fs.FileInfo{}

	err1 := filepath.Walk(file_path, func(fpath string, info fs.FileInfo, err error) error {
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			return nil
		}
		for _, ptn := range path_exclude_ptn {
			if ptn.MatchString(fpath) {
				if *debug {
					fmt.Fprintf(os.Stderr, "SKIP PATH %s\n", fpath)
				}
				return nil
			}
		}
		fname := info.Name()
		if info.IsDir() && ((excludePtn != nil && excludePtn.MatchString(fname)) || (defaultExcludePtn != nil && defaultExcludePtn.MatchString(fname))) {
			if *debug {
				fmt.Fprintf(os.Stderr, "SKIP DIR %s\n", fpath)
			}
			return filepath.SkipDir
		}
		// Check if the file matches the pattern

		if !info.IsDir() {
			total_files_scanned++
			if fpath != *load_profile_path && filename_regexp.MatchString(fname) && ((excludePtn == nil) || (excludePtn != nil && !excludePtn.MatchString(fname))) && ((defaultExcludePtn == nil) || (defaultExcludePtn != nil && !defaultExcludePtn.MatchString(fname))) {
				if *skipBinary {
					isbin, err := u.IsBinaryFileSimple(fpath)
					if (err == nil) && isbin {
						if *debug {
							fmt.Fprintf(os.Stderr, "SKIP BIN %s\n", fpath)
						}
						return nil
					}
				}

				fmode := info.Mode()
				if !(fmode.IsRegular()) {
					return nil
				}
				if len(filesBatch) < *batchSize {
					if *debug {
						fmt.Fprintf(os.Stderr, "Add file: %s\n", fpath)
					}
					filesBatch[fpath] = info
				} else {
					wg.Add(1)
					go cred_detect_ProcessFiles(&wg, filesBatch, *password_check_mode, 0, output_chan, log_chan, *debug)
					filesBatch = map[string]fs.FileInfo{fpath: info} // Need to add this one as the batch is full we miss add it.
				}
			}
		}
		return nil
	})

	if len(filesBatch) > 0 { // Last batch
		wg.Add(1)
		go cred_detect_ProcessFiles(&wg, filesBatch, *password_check_mode, 0, output_chan, log_chan, *debug)
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
	fmt.Fprintf(os.Stderr, "Scanned %d files and has processed %d files\n", total_files_scanned, total_files_process)
}
