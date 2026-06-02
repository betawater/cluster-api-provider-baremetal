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
	"encoding/json"
	"fmt"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	

	cfov1 "github.com/BetaWater/cluster-api-provider-baremetal/modules/cvo/api/v1beta1"
)

// MergeValues merges default values with user-provided values.
// User values take precedence over defaults.
func MergeValues(defaults, overrides map[string]apiextensionsv1.JSON) map[string]interface{} {
	result := make(map[string]interface{})

	// Apply defaults
	for k, v := range defaults {
		var val interface{}
		if err := json.Unmarshal(v.Raw, &val); err == nil {
			result[k] = val
		}
	}

	// Apply overrides (deep merge)
	for k, v := range overrides {
		var val interface{}
		if err := json.Unmarshal(v.Raw, &val); err == nil {
			result[k] = deepMerge(result[k], val)
		}
	}

	return result
}

// deepMerge recursively merges two values.
func deepMerge(base, override interface{}) interface{} {
	baseMap, baseOk := base.(map[string]interface{})
	overrideMap, overrideOk := override.(map[string]interface{})

	if !baseOk || !overrideOk {
		return override
	}

	result := make(map[string]interface{})
	for k, v := range baseMap {
		result[k] = v
	}
	for k, v := range overrideMap {
		if existing, ok := result[k]; ok {
			result[k] = deepMerge(existing, v)
		} else {
			result[k] = v
		}
	}
	return result
}

// ValidateVariables validates user-provided values against variable definitions.
func ValidateVariables(values map[string]apiextensionsv1.JSON, vars []cfov1.AddonVariable) error {
	// Check required variables
	for _, v := range vars {
		if v.Required {
			if _, ok := values[v.Name]; !ok {
				return fmt.Errorf("required variable %q is not provided", v.Name)
			}
		}
	}

	// Validate types and enum values
	for name, value := range values {
		varDef := findVariable(vars, name)
		if varDef == nil {
			continue // Unknown variables are allowed (passed through)
		}

		// Type validation
		var val interface{}
		if err := json.Unmarshal(value.Raw, &val); err != nil {
			return fmt.Errorf("invalid value for variable %q: %w", name, err)
		}

		if err := validateType(val, varDef.Type); err != nil {
			return fmt.Errorf("invalid type for variable %q: %w", name, err)
		}

		// Enum validation
		if len(varDef.Enum) > 0 {
			if !containsEnumValue(varDef.Enum, value) {
				return fmt.Errorf("value for variable %q not in allowed values", name)
			}
		}
	}

	return nil
}

// findVariable finds a variable definition by name.
func findVariable(vars []cfov1.AddonVariable, name string) *cfov1.AddonVariable {
	for i := range vars {
		if vars[i].Name == name {
			return &vars[i]
		}
	}
	return nil
}

// validateType validates a value against the expected type.
func validateType(val interface{}, expectedType cfov1.VariableType) error {
	switch expectedType {
	case cfov1.VariableTypeString:
		if _, ok := val.(string); !ok {
			return fmt.Errorf("expected string, got %T", val)
		}
	case cfov1.VariableTypeNumber:
		if _, ok := val.(float64); !ok {
			return fmt.Errorf("expected number, got %T", val)
		}
	case cfov1.VariableTypeBoolean:
		if _, ok := val.(bool); !ok {
			return fmt.Errorf("expected boolean, got %T", val)
		}
	case cfov1.VariableTypeObject:
		if _, ok := val.(map[string]interface{}); !ok {
			return fmt.Errorf("expected object, got %T", val)
		}
	}
	return nil
}

// containsEnumValue checks if a value is in the enum list.
func containsEnumValue(enum []apiextensionsv1.JSON, value apiextensionsv1.JSON) bool {
	for _, e := range enum {
		if string(e.Raw) == string(value.Raw) {
			return true
		}
	}
	return false
}
