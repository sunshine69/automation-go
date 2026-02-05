package lib

import (
	"bytes"
	"fmt"

	"github.com/CloudyKit/jet/v6"
)

// WIP - just new idea, dont use anything here

// JetTemplateString takes a raw template string, a data map, and returns the rendered output.
func JetTemplateString(templateContent string, data map[string]any) (string, error) {
	// 1. Create an In-Memory loader to hold our string content
	// We give the "file" a virtual name (e.g., "my_template.jet")
	loader := jet.NewInMemLoader()
	loader.Set("my_template.jet", templateContent)

	// 2. Create the Jet Set with your custom delimiters
	views := jet.NewSet(
		loader,
		// jet.WithDelims("{$", "$}"),
	)

	// 3. Get the template from the virtual loader
	t, err := views.GetTemplate("my_template.jet")
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	// 4. Render the template into a buffer
	var buf bytes.Buffer
	err = t.Execute(&buf, nil, data)
	if err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

func RenderJetTemplate(
	templateStr string,
	vars map[string]any,
	leftDelim string,
	rightDelim string,
) (string, error) {

	if leftDelim == "" {
		leftDelim = "{{"
	}
	if rightDelim == "" {
		rightDelim = "}}"
	}

	loader := jet.NewInMemLoader()

	set := jet.NewSet(
		loader,
		jet.WithDelims(leftDelim, rightDelim),
	)

	set.AddGlobal("add", AddAny)

	tpl, err := set.Parse("inline", templateStr)
	if err != nil {
		return "", err
	}

	jv := make(jet.VarMap)
	for k, v := range vars {
		jv.Set(k, v)
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, jv, nil); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func AddAny(a, b int) (int, error) {
	return (a + b), nil
}
