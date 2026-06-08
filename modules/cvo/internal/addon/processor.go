/*
Copyright 2024 The CAPBM Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package addon

import (
	"bytes"
	"text/template"

	"sigs.k8s.io/yaml"
)

// ManifestProcessor processes manifest templates with variables.
type ManifestProcessor struct{}

// Process substitutes variables in a manifest template.
// The template uses Go template syntax with .Vars for variable access.
// Example: {{ .Vars.appName }} or {{ .Vars.replicas }}
func (p *ManifestProcessor) Process(content []byte, variables map[string]interface{}) ([]byte, error) {
	// Create template with variables
	tmpl, err := template.New("manifest").
		Option("missingkey=error").
		Funcs(template.FuncMap{
			"toYaml": toYAML,
			"indent": indent,
		}).
		Parse(string(content))
	if err != nil {
		return nil, err
	}

	// Execute template
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]interface{}{
		"Vars": variables,
	}); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// toYAML converts a value to YAML string.
func toYAML(v interface{}) string {
	data, err := yaml.Marshal(v)
	if err != nil {
		return ""
	}
	return string(data)
}

// indent adds indentation to a string.
func indent(n int, s string) string {
	prefix := ""
	for i := 0; i < n; i++ {
		prefix += " "
	}
	return prefix + s
}
