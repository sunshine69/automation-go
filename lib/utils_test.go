package lib

import (
	"os"
	"testing"
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
