package lib

import (
	"os"
	"regexp"
	"strconv"
	"testing"

	u "github.com/sunshine69/golang-tools/utils"
)

var project_dir string

func init() {
	project_dir, _ = os.Getwd()
}

func BenchmarkTemplateString(b *testing.B) {
	for n := 0; n < b.N; n++ {
		TemplateString(`<?php  var2 - {{var2}} this is output {{ var1 |join(",")}} - ?>`, map[string]any{"var1": []string{"a", "b", "c"}, "var2": "Value var2"})
	}
}

func TestJinja2(t *testing.T) {
	TemplateFile("../tmp/test.j2", "../tmp/test.txt", map[string]interface{}{"header": "Header", "lines": []string{"line1", "line2", "line3"}}, 0o777)
	TemplateFile("../tmp/test1.j2", "../tmp/test1.txt", map[string]interface{}{"header": "Header", "lines": []string{"line1", "line2", "line3"}}, 0o777)
	dataStr := `This is simple {{ newvar }}`
	println(TemplateString(dataStr, map[string]any{"newvar": "New value of new var"}))
	dataStr = `#jinja2:variable_start_string:'{$', variable_end_string:'$}', trim_blocks:True, lstrip_blocks:True
This is has config line {{ newvar }} and {$ newvar $}`
	println(TemplateString(dataStr, map[string]any{"newvar": "New value of new var"}))

	u.GoTemplateFile("../tmp/test.go.tmpl", "../tmp/test.go.txt", map[string]interface{}{"header": "Header", "lines": []string{"line1", "line2", "line3"}}, 0o777)
	u.GoTemplateFile("../tmp/test1.go.tmpl", "../tmp/test1.go.txt", map[string]interface{}{"header": "Header", "lines": []string{"line1", "line2", "line3"}}, 0o777)
	data := IncludeVars("/home/sitsxk5/src/Sonic.Commercial.Ordering/azure-devops/vars-ansible.yaml")
	// u.GoTemplateFile("/home/sitsxk5/tmp/all.yaml", "/home/sitsxk5/tmp/test.yaml",
	// 	data, 0644)
	// data := map[string]any{"packages": []string{"p1", "p2", "p3"}}
	// New line after the coma makes it rendered properly - strange but keep this result as a sample
	o := TemplateString(`#jinja2:variable_start_string:'{$', variable_end_string:'$}', trim_blocks:True, lstrip_blocks:True
	[
			{% for app in packages %}
			"{$ app $}_config-pkg",
			"{$ app $}"{% if not loop.last %},
			{% endif %}
			{% endfor %}
			]`, data)

	println(o)

	o = u.GoTemplateString(`#gotmpl:variable_start_string:'{$', variable_end_string:'$}'
	[
			{{ range $idx, $app := .packages -}}
			"{{ $app }}_config-pkg",
			"{{ $app }}"{{ if ne $idx (add (len $.packages) -1) }},{{ end }}
			{{ end -}}
			]`, data)

	println(o)
}

func TestAdhoc(t *testing.T) {
	// u.ExtractTextBlock("/home/sitsxk5/src/Sonic.TCM.Web/pages/171/edit_form.php")
	filename := "/home/sitsxk5/src/Sonic.TCM.Web/pages/171/edit_form.php"
	ptn := regexp.MustCompile(`(?m)\<\?php echo (\$[a-zA-Z0-9]+); \?\>`)
	datab, err := os.ReadFile(filename)
	u.CheckErr(err, "")
	newdata := ptn.ReplaceAllString(string(datab), `<?php echo htmlspecialchars($1);`)
	u.CheckErr(os.WriteFile(filename, []byte(newdata), 0o777), "Write faile")

	lines := u.PickLinesInFile(filename, 64, 65)
	for _, l := range lines {
		println(l)
	}
}

func TestPasswordDetect(t *testing.T) {
	p := "1q2w3e"
	if !u.Exists("/tmp/words.txt") {
		u.Curl("GET", "https://devops-tools.au.int.sonichealthcare/smb/get?path=Downloads/words.txt", "", "/tmp/words.txt", []string{})
	}

	token := os.Getenv("TFS_TOKEN")
	o := u.Must(u.Curl("GET", `https://:`+token+`@tfs.sonichealth.com.au/SonicHealth/DevOps/_apis/git/pullrequests?searchCriteria.status=completed&\$top=2&api-version=6.0`, "", "", []string{}))

	u.Curl("GET", "https://devops-tools.au.int.sonichealthcare/smb/get?path=stevek/gotools.tgz", "", "", []string{})

	myob := u.JsonToMap(o)

	// println(u.JsonDump(myob, ""))
	values := myob["value"].([]any)
	_ = u.Must(strconv.ParseInt("qwee12222344", 10, 64))

	for _, i := range values {
		x := i.(map[string]any)
		println(u.JsonDump(x, ""))
	}
	if IsLikelyPasswordOrToken(p, "letter+word", "/tmp/words.txt", 0, 0) {
		println("Is password!!!")
	}
}

func TestIniHandling(t *testing.T) {
	IniSetVal("test.ini", "global", "tfs_token", "aaaaaa")
}
