package lib

import (
	"fmt"
	"os"
	"regexp"
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
	data := map[string]any{"packages": []string{"p1", "p2", "p3"}}
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
	// u.ExtractTextBlock("/home/sitsxk5/src")
	filename := "/home/stevek/src/"
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
	p := "1Password!"
	if !u.Exists("/tmp/words.txt") {
		u.Curl("GET", "https://github.com/dwyl/english-words/blob/master/words.txt", "", "/tmp/words.txt", []string{}, nil)
	}

	if IsLikelyPasswordOrToken(p, "letter+word", "/tmp/words.txt", 0, 0) {
		println("Is password!!!")
	}
}

func TestIniHandling(t *testing.T) {
	IniSetVal("test.ini", "global", "tfs_token", "aaaaaa")
}

func TestCheckPasswordStrength_TooShort(t *testing.T) {
	password := "abc123" // 6 characters
	strength, entropy, err := CheckPasswordStrength(password)
	fmt.Println(strength, entropy)
	if strength != WeakPassword || err == nil {
		t.Errorf("Expected WeakPassword and error for short password (6 chars), got: %s, %v", strength, err)
	}
}

func TestCheckPasswordStrength_AllLowercase(t *testing.T) {
	password := "abcdefg" // 7 characters
	strength, entropy, err := CheckPasswordStrength(password)
	fmt.Println(strength, entropy)
	if strength != WeakPassword || err == nil {
		t.Errorf("Expected WeakPassword and error for single-type password (all lowercase), got: %s, %v", strength, err)
	}
}

func TestCheckPasswordStrength_AllUppercase(t *testing.T) {
	password := "ABCDEFG" // 7 characters
	strength, entropy, err := CheckPasswordStrength(password)
	fmt.Println(strength, entropy)
	if strength != WeakPassword || err == nil {
		t.Errorf("Expected WeakPassword and error for single-type password (all uppercase), got: %s, %v", strength, err)
	}
}

func TestCheckPasswordStrength_AllDigits(t *testing.T) {
	password := "12345678" // 8 characters
	strength, entropy, err := CheckPasswordStrength(password)
	fmt.Println(strength, entropy)
	if strength != WeakPassword || err == nil {
		t.Errorf("Expected WeakPassword and error for single-type password (all digits), got: %s, %v", strength, err)
	}
}

func TestCheckPasswordStrength_AllSymbols(t *testing.T) {
	password := "!@#$%^&*" // 8 characters
	strength, entropy, err := CheckPasswordStrength(password)
	fmt.Println(strength, entropy)
	if strength != WeakPassword || err == nil {
		t.Errorf("Expected WeakPassword and error for single-type password (all symbols), got: %s, %v", strength, err)
	}
}

func TestCheckPasswordStrength_MixedButLowEntropy(t *testing.T) {
	password := "Aa1!Aa1!" // 8 characters, mixed but predictable
	strength, entropy, err := CheckPasswordStrength(password)
	fmt.Println(strength, entropy)
	if strength != MediumPassword || err == nil {
		t.Errorf("Expected MediumPassword and no error for mixed password with low entropy, got: %s, %v", strength, err)
	}
}

func TestCheckPasswordStrength_StrongPassword(t *testing.T) {
	password := "Abcdefg123!@#"
	strength, entropy, err := CheckPasswordStrength(password)
	fmt.Println(strength, entropy)
	if strength != StrongPassword || err == nil {
		t.Errorf("Expected StrongPassword and no error for a strong password, got: %s, %v", strength, err)
	}
}

func TestCheckPasswordStrength_VeryStrongPassword(t *testing.T) {
	password := "tZrQN|7YIfk+=JF.d%2hIb1j*E=Gszc~x5d-"
	strength, entropy, err := CheckPasswordStrength(password)

	fmt.Println(strength, entropy)
	if strength != VeryStrongPassword {
		t.Errorf("Expected Very StrongPassword and no error for a very strong password, got: %s, %v", strength, err)
	}
}

func TestCheckPasswordStrength_MixedTypesNoError(t *testing.T) {
	password := "Abc123!@#"
	strength, entropy, err := CheckPasswordStrength(password)

	fmt.Println(strength, entropy)
	if strength != MediumPassword || err == nil {
		t.Errorf("Expected MediumPassword and no error for mixed-type password, got: %s, %v", strength, err)
	}
}

func TestCalculateEntropy(t *testing.T) {
	password := "Abcdefg123!@#"
	entropy := CalculateEntropy(password)
	fmt.Println(entropy)

}

var testPasswords = []string{
	"aaaaa",                              // Short and repetitive
	"abc123!@#",                          // Mixed character types (short)
	"verylongpasswordwithmixedchars123!", // Longer, mixed characters
}

func BenchmarkCalculateEntropy(b *testing.B) {
	for _, pwd := range testPasswords {
		b.Run(pwd, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				CalculateEntropy(pwd)
			}
		})
	}
}

func TestFlattenVar(t *testing.T) {
	vault_data := u.Must(u.Encrypt("my-password", "1qa2ws", u.DefaultEncryptionConfig()))
	data := map[string]any{
		"var1": "{{ var2 | upper }}",
		"var2": "<vault>" + vault_data + "</vault> value3 as int: {{ var3 }}",
		"var3": 234,
		"var4": "[\"a\", \"b - {{ var2 }}\"]",
		"var5": "{\"b\": [\"c\", 234]}",
	}
	os.Setenv("VAULT_PASSWORD", "1qa2ws")
	vars := u.Must(FlattenAllVars(data))
	fmt.Println(u.JsonDump(vars, ""))
}
