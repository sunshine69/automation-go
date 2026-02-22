// benchmark_test.go
package lib

// 1. Simple Loop
// Go Template: 100% (Baseline)
// Jinja2 (Go port): ~68% slower (takes 1.68x longer)
// Gonja: ~236% slower (takes 3.36x longer)
// 2. Complex Rendering
// Go Template: 100% (Baseline)
// Jinja2 (Go port): ~39% slower (takes 1.39x longer)
// Gonja: ~292% slower (takes 3.92x longer)
// 3. With Conditionals
// Go Template: 100% (Baseline)
// Jinja2 (Go port): ~197% slower (takes 2.97x longer)
// Gonja: ~370% slower (takes 4.70x longer)
// 4. With Functions
// Go Template: 100% (Baseline)
// Jinja2 (Go port): ~77% slower (takes 1.77x longer)
// Gonja: ~149% slower (takes 2.49x longer)

// So minijinja2 is on par or a bit slower than Pongo but without the bug nd not able to change var start/end like pongo
// in my test last time

import (
	"github.com/sunshine69/golang-tools/utils"

	"testing"
	"time"

	"github.com/sunshine69/automation-go/gonja" // Adjust this import path
)

// Sample data for benchmarking
var (
	// Large dataset for loop rendering
	largeData = make([]map[string]interface{}, 1000)
	// Complex data for high cost rendering
	complexData = map[string]interface{}{
		"users": func() []map[string]interface{} {
			users := make([]map[string]interface{}, 100)
			for i := 0; i < 100; i++ {
				users[i] = map[string]interface{}{
					"id":       i,
					"name":     "User " + string(rune('A'+i%26)),
					"email":    "user" + string(rune('A'+i%26)) + "@example.com",
					"metadata": map[string]interface{}{"created": time.Now().Format("2006-01-02"), "active": i%2 == 0},
					"tags":     []string{"tag1", "tag2", "tag3"},
				}
			}
			return users
		}(),
		"config": map[string]interface{}{
			"version":  "1.0.0",
			"features": []string{"feature1", "feature2", "feature3", "feature4", "feature5"},
			"settings": map[string]interface{}{
				"debug":   true,
				"timeout": 30,
				"retries": 3,
			},
		},
		"timestamp": time.Now().Format("2006-01-02 15:04:05"),
		"total":     1000,
		"active":    true,
	}
)

func init() {
	// Initialize large data
	for i := 0; i < 1000; i++ {
		largeData[i] = map[string]interface{}{
			"id":    i,
			"name":  "Item " + string(rune('A'+i%26)),
			"value": i * 10,
		}
	}
}

// BenchmarkGoTemplateSimpleLoop benchmarks simple loop rendering with Go template
func BenchmarkGoTemplateSimpleLoop(b *testing.B) {
	tmpl := `{{range .items}}{{.id}}:{{.name}}:{{.value}}{{end}}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = utils.GoTemplateString(tmpl, map[string]interface{}{
			"items": largeData,
		})
	}
}

// BenchmarkJinja2SimpleLoop benchmarks simple loop rendering with Jinja2-style template
func BenchmarkJinja2SimpleLoop(b *testing.B) {
	tmpl := `{% for item in items %}{{ item.id }}:{{ item.name }}:{{ item.value }}{% endfor %}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = TemplateString(tmpl, map[string]interface{}{
			"items": largeData,
		})
	}
}

// BenchmarkGonjaSimpleLoop benchmarks simple loop rendering with gonja
func BenchmarkGonjaSimpleLoop(b *testing.B) {
	tmpl := `{% for item in items %}{{ item.id }}:{{ item.name }}:{{ item.value }}{% endfor %}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = gonja.TemplateString(tmpl, map[string]interface{}{
			"items": largeData,
		})
	}
}

// BenchmarkGoTemplateComplexRendering benchmarks complex rendering with Go template
func BenchmarkGoTemplateComplexRendering(b *testing.B) {
	tmpl := `{{range .users}}<div class="user">
  <h3>{{.name}}</h3>
  <p>Email: {{.email}}</p>
  <p>ID: {{.id}}</p>
  <p>Active: {{.metadata.active}}</p>
  <p>Created: {{.metadata.created}}</p>
  <ul>
  {{range .tags}}<li>{{.}}</li>{{end}}
  </ul>
</div>{{end}}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = utils.GoTemplateString(tmpl, complexData)
	}
}

// BenchmarkJinja2ComplexRendering benchmarks complex rendering with Jinja2-style template
func BenchmarkJinja2ComplexRendering(b *testing.B) {
	tmpl := `{% for user in users %}<div class="user">
  <h3>{{ user.name }}</h3>
  <p>Email: {{ user.email }}</p>
  <p>ID: {{ user.id }}</p>
  <p>Active: {{ user.metadata.active }}</p>
  <p>Created: {{ user.metadata.created }}</p>
  <ul>
  {% for tag in user.tags %}<li>{{ tag }}</li>{% endfor %}
  </ul>
</div>{% endfor %}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = TemplateString(tmpl, complexData)
	}
}

// BenchmarkGonjaComplexRendering benchmarks complex rendering with gonja
func BenchmarkGonjaComplexRendering(b *testing.B) {
	tmpl := `{% for user in users %}<div class="user">
  <h3>{{ user.name }}</h3>
  <p>Email: {{ user.email }}</p>
  <p>ID: {{ user.id }}</p>
  <p>Active: {{ user.metadata.active }}</p>
  <p>Created: {{ user.metadata.created }}</p>
  <ul>
  {% for tag in user.tags %}<li>{{ tag }}</li>{% endfor %}
  </ul>
</div>{% endfor %}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = gonja.TemplateString(tmpl, complexData)
	}
}

// BenchmarkGoTemplateWithConditionals benchmarks conditional rendering with Go template
func BenchmarkGoTemplateWithConditionals(b *testing.B) {
	tmpl := `{{range .users}}{{if .metadata.active}}<div class="active-user">{{.name}}</div>{{else}}<div class="inactive-user">{{.name}}</div>{{end}}{{end}}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = utils.GoTemplateString(tmpl, complexData)
	}
}

// BenchmarkJinja2WithConditionals benchmarks conditional rendering with Jinja2-style template
func BenchmarkJinja2WithConditionals(b *testing.B) {
	tmpl := `{% for user in users %}{% if user.metadata.active %}<div class="active-user">{{ user.name }}</div>{% else %}<div class="inactive-user">{{ user.name }}</div>{% endif %}{% endfor %}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = TemplateString(tmpl, complexData)
	}
}

// BenchmarkGonjaWithConditionals benchmarks conditional rendering with gonja
func BenchmarkGonjaWithConditionals(b *testing.B) {
	tmpl := `{% for user in users %}{% if user.metadata.active %}<div class="active-user">{{ user.name }}</div>{% else %}<div class="inactive-user">{{ user.name }}</div>{% endif %}{% endfor %}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = gonja.TemplateString(tmpl, complexData)
	}
}

// BenchmarkGoTemplateWithFunctions benchmarks template functions with Go template
func BenchmarkGoTemplateWithFunctions(b *testing.B) {
	tmpl := `{{range .users}}<div class="user">{{.name|upper}}</div>{{end}}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = utils.GoTemplateString(tmpl, complexData)
	}
}

// BenchmarkJinja2WithFunctions benchmarks template functions with Jinja2-style template
func BenchmarkJinja2WithFunctions(b *testing.B) {
	tmpl := `{% for user in users %}<div class="user">{{ user.name|upper }}</div>{% endfor %}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = TemplateString(tmpl, complexData)
	}
}

// BenchmarkGonjaWithFunctions benchmarks template functions with gonja
func BenchmarkGonjaWithFunctions(b *testing.B) {
	tmpl := `{% for user in users %}<div class="user">{{ user.name.upper() }}</div>{% endfor %}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = gonja.TemplateString(tmpl, complexData)
	}
}
