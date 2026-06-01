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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	infrav1 "github.com/BetaWater/cluster-api-provider-baremetal/api/v1beta1"
)

func TestFindAddonDefinition(t *testing.T) {
	release := &infrav1.ReleaseImage{
		Spec: infrav1.ReleaseImageSpec{
			Addons: []infrav1.AddonDefinition{
				{Name: "capi-core-controller", Version: "v1.7.0"},
				{Name: "kubeadm-bootstrap", Version: "v1.7.0"},
			},
		},
	}

	tests := []struct {
		name     string
		addon    string
		expected string
	}{
		{"found", "capi-core-controller", "v1.7.0"},
		{"not found", "nonexistent", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := findAddonDefinition(release, tt.addon)
			if tt.expected == "" {
				if def != nil {
					t.Errorf("expected nil, got %+v", def)
				}
				return
			}
			if def == nil {
				t.Errorf("expected definition, got nil")
				return
			}
			if def.Version != tt.expected {
				t.Errorf("expected version %s, got %s", tt.expected, def.Version)
			}
		})
	}
}

func TestFindAddonDefinitionNilRelease(t *testing.T) {
	def := findAddonDefinition(nil, "test")
	if def != nil {
		t.Errorf("expected nil for nil release, got %+v", def)
	}
}

func TestAddonUpgradeStrategyDefaults(t *testing.T) {
	strategy := &infrav1.AddonUpgradeStrategy{
		Type:       "Rolling",
		RetryCount: 3,
		Timeout:    &metav1.Duration{Duration: 300000000000},
	}

	if strategy.Type != "Rolling" {
		t.Errorf("expected type Rolling, got %s", strategy.Type)
	}
	if strategy.RetryCount != 3 {
		t.Errorf("expected retry count 3, got %d", strategy.RetryCount)
	}
}

func TestAddonInstallStrategyDefaults(t *testing.T) {
	strategy := &infrav1.AddonInstallStrategy{
		Timeout:         &metav1.Duration{Duration: 300000000000},
		RetryCount:      3,
		CreateNamespace: true,
		Wait:            true,
	}

	if !strategy.CreateNamespace {
		t.Errorf("expected CreateNamespace true")
	}
	if !strategy.Wait {
		t.Errorf("expected Wait true")
	}
}

func TestAddonHookOnFailure(t *testing.T) {
	tests := []struct {
		name      string
		onFailure string
	}{
		{"abort", "Abort"},
		{"continue", "Continue"},
		{"ignore", "Ignore"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook := infrav1.AddonHook{
				Name:      "test-hook",
				Command:   "echo test",
				OnFailure: tt.onFailure,
			}
			if hook.OnFailure != tt.onFailure {
				t.Errorf("expected OnFailure %s, got %s", tt.onFailure, hook.OnFailure)
			}
		})
	}
}

func TestAddonDefinitionWithAllFields(t *testing.T) {
	addonDef := infrav1.AddonDefinition{
		Name:        "capi-core-controller",
		Type:        infrav1.AddonTypeHelm,
		Version:     "v1.7.0",
		ContentPath: "charts/capi-core-controller-v1.7.0.tgz",
		Namespace:   "capi-system",
		Dependencies: []string{},
		InstallStrategy: &infrav1.AddonInstallStrategy{
			Timeout:         &metav1.Duration{Duration: 300000000000},
			RetryCount:      3,
			CreateNamespace: true,
			Wait:            true,
		},
		UpgradeStrategy: &infrav1.AddonUpgradeStrategy{
			Type:           "Rolling",
			MaxUnavailable: 0,
			Timeout:        &metav1.Duration{Duration: 300000000000},
			RetryCount:     3,
		},
		PreHooks: []infrav1.AddonHook{
			{
				Name:      "validate-crds",
				Command:   "kubectl get crd clusters.cluster.x-k8s.io",
				Timeout:   &metav1.Duration{Duration: 30000000000},
				OnFailure: "Abort",
			},
		},
		PostHooks: []infrav1.AddonHook{
			{
				Name:      "verify-webhook",
				Command:   "kubectl get service capi-webhook-service -n capi-system",
				Timeout:   &metav1.Duration{Duration: 60000000000},
				OnFailure: "Abort",
			},
		},
		Upgrade: &infrav1.ComponentUpgradeConfig{
			Backup: infrav1.ComponentBackupConfig{
				Enabled: true,
				Config: []infrav1.BackupItem{
					{Path: "/etc/capi-core-controller", Type: "directory"},
				},
			},
			Rollback: infrav1.ComponentRollbackConfig{
				Script:  "scripts/rollback-capi-core.sh",
				Timeout: &metav1.Duration{Duration: 300000000000},
			},
			HealthCheck: infrav1.ComponentHealthCheckConfig{
				Command: "kubectl get deployment capi-controller-manager -n capi-system",
				Timeout: &metav1.Duration{Duration: 60000000000},
				Retries: 3,
			},
		},
	}

	if addonDef.Name != "capi-core-controller" {
		t.Errorf("expected name capi-core-controller, got %s", addonDef.Name)
	}
	if addonDef.InstallStrategy == nil {
		t.Errorf("expected InstallStrategy to be set")
	}
	if addonDef.UpgradeStrategy == nil {
		t.Errorf("expected UpgradeStrategy to be set")
	}
	if len(addonDef.PreHooks) != 1 {
		t.Errorf("expected 1 pre-hook, got %d", len(addonDef.PreHooks))
	}
	if len(addonDef.PostHooks) != 1 {
		t.Errorf("expected 1 post-hook, got %d", len(addonDef.PostHooks))
	}
	if addonDef.Upgrade == nil {
		t.Errorf("expected Upgrade to be set")
	}
}
