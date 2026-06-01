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

package upgrader

import infrav1 "github.com/BetaWater/cluster-api-provider-baremetal/api/v1beta1"

// ComponentDiff records component version changes between two ReleaseImages.
type ComponentDiff struct {
	Changed   []ComponentChange `json:"changed"`
	Unchanged []string          `json:"unchanged"`
	Added     []string          `json:"added"`
	Removed   []string          `json:"removed"`
}

// ComponentChange records a single component version change.
type ComponentChange struct {
	Name           string `json:"name"`
	CurrentVersion string `json:"currentVersion"`
	TargetVersion  string `json:"targetVersion"`
}

// DiffComponents compares the components of two ReleaseImages and returns a diff.
// It does NOT use shell commands -- all version info comes from ReleaseImage.Spec.Components.
func DiffComponents(current, target *infrav1.ReleaseImage) *ComponentDiff {
	if current == nil || target == nil {
		return &ComponentDiff{}
	}

	diff := &ComponentDiff{}

	// Compare containerd
	currentContainerd := current.Spec.Components.Containerd.Version
	targetContainerd := target.Spec.Components.Containerd.Version
	if currentContainerd != targetContainerd {
		if targetContainerd != "" {
			diff.Changed = append(diff.Changed, ComponentChange{
				Name:           "containerd",
				CurrentVersion: currentContainerd,
				TargetVersion:  targetContainerd,
			})
		}
	} else if currentContainerd != "" {
		diff.Unchanged = append(diff.Unchanged, "containerd")
	}

	// Compare kubernetes version
	currentK8s := current.Spec.Components.Kubernetes.Version
	targetK8s := target.Spec.Components.Kubernetes.Version
	if currentK8s != targetK8s {
		if targetK8s != "" {
			diff.Changed = append(diff.Changed, ComponentChange{
				Name:           "kubernetes",
				CurrentVersion: currentK8s,
				TargetVersion:  targetK8s,
			})
		}
	} else if currentK8s != "" {
		diff.Unchanged = append(diff.Unchanged, "kubernetes")
	}

	// Compare all addons (including CAPI Core)
	currentAddonMap := buildAddonMap(current)
	targetAddonMap := buildAddonMap(target)

	for name, targetAddon := range targetAddonMap {
		currentAddon, exists := currentAddonMap[name]
		if !exists {
			diff.Added = append(diff.Added, name)
			continue
		}

		if currentAddon.Version != targetAddon.Version {
			if targetAddon.Version != "" {
				diff.Changed = append(diff.Changed, ComponentChange{
					Name:           name,
					CurrentVersion: currentAddon.Version,
					TargetVersion:  targetAddon.Version,
				})
			}
		} else if currentAddon.Version != "" {
			diff.Unchanged = append(diff.Unchanged, name)
		}
	}

	for name := range currentAddonMap {
		if _, exists := targetAddonMap[name]; !exists {
			diff.Removed = append(diff.Removed, name)
		}
	}

	return diff
}

// findAddonByName finds an addon by name in the ReleaseImage.
func findAddonByName(ri *infrav1.ReleaseImage, name string) *infrav1.AddonDefinition {
	for i := range ri.Spec.Addons {
		if ri.Spec.Addons[i].Name == name {
			return &ri.Spec.Addons[i]
		}
	}
	return nil
}

// buildAddonMap builds a map of addon name to addon definition.
func buildAddonMap(ri *infrav1.ReleaseImage) map[string]*infrav1.AddonDefinition {
	result := make(map[string]*infrav1.AddonDefinition)
	for i := range ri.Spec.Addons {
		result[ri.Spec.Addons[i].Name] = &ri.Spec.Addons[i]
	}
	return result
}

// getComponentVersionByName returns the version of a named component from a ReleaseImage.
func getComponentVersionByName(ri *infrav1.ReleaseImage, name string) string {
	switch name {
	case "containerd":
		return ri.Spec.Components.Containerd.Version
	case "kubernetes":
		return ri.Spec.Components.Kubernetes.Version
	default:
		// Check all addons
		addonMap := buildAddonMap(ri)
		if addon, exists := addonMap[name]; exists {
			return addon.Version
		}
		return ""
	}
}

// NeedsUpgrade returns true if any component needs to be upgraded.
func (d *ComponentDiff) NeedsUpgrade() bool {
	return len(d.Changed) > 0 || len(d.Added) > 0
}

// UpgradeSet returns the set of component names that need upgrading.
func (d *ComponentDiff) UpgradeSet() map[string]bool {
	needs := make(map[string]bool)
	for _, c := range d.Changed {
		needs[c.Name] = true
	}
	for _, name := range d.Added {
		needs[name] = true
	}
	return needs
}
