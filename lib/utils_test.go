package lib

import (
	"os"
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
	u.GoTemplateFile("../tmp/test.go.tmpl", "../tmp/test.go.txt", map[string]interface{}{"header": "Header", "lines": []string{"line1", "line2", "line3"}}, 0o777)
	TemplateFile("../tmp/test1.j2", "../tmp/test1.txt", map[string]interface{}{"header": "Header", "lines": []string{"line1", "line2", "line3"}}, 0o777)
	u.GoTemplateFile("../tmp/test1.go.tmpl", "../tmp/test1.go.txt", map[string]interface{}{"header": "Header", "lines": []string{"line1", "line2", "line3"}}, 0o777)
}
