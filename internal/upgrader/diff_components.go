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

	// Compare CNI/CSI components
	cniComponents := map[string]string{
		"calico":  target.Spec.Components.Calico.Version,
		"cilium":  target.Spec.Components.Cilium.Version,
		"cephCsi": target.Spec.Components.CephCsi.Version,
	}
	for name, targetVer := range cniComponents {
		currentVer := getComponentVersionByName(current, name)
		if currentVer != targetVer && targetVer != "" {
			diff.Changed = append(diff.Changed, ComponentChange{
				Name:           name,
				CurrentVersion: currentVer,
				TargetVersion:  targetVer,
			})
		} else if targetVer != "" {
			diff.Unchanged = append(diff.Unchanged, name)
		}
	}

	return diff
}

// getComponentVersionByName returns the version of a named component from a ReleaseImage.
func getComponentVersionByName(ri *infrav1.ReleaseImage, name string) string {
	switch name {
	case "calico":
		return ri.Spec.Components.Calico.Version
	case "cilium":
		return ri.Spec.Components.Cilium.Version
	case "cephCsi":
		return ri.Spec.Components.CephCsi.Version
	case "containerd":
		return ri.Spec.Components.Containerd.Version
	case "kubernetes":
		return ri.Spec.Components.Kubernetes.Version
	default:
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
