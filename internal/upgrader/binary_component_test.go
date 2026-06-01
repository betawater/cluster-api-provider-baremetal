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

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	infrav1 "github.com/BetaWater/cluster-api-provider-baremetal/api/v1beta1"
)

func TestBinaryInstallStrategyDefaults(t *testing.T) {
	strategy := &infrav1.BinaryInstallStrategy{
		Timeout:     &metav1.Duration{Duration: 300000000000},
		RetryCount:  3,
		Method:      "package",
		ServiceName: "containerd",
	}

	if strategy.RetryCount != 3 {
		t.Errorf("expected retry count 3, got %d", strategy.RetryCount)
	}
	if strategy.Method != "package" {
		t.Errorf("expected method package, got %s", strategy.Method)
	}
	if strategy.ServiceName != "containerd" {
		t.Errorf("expected service name containerd, got %s", strategy.ServiceName)
	}
}

func TestBinaryUpgradeStrategyDefaults(t *testing.T) {
	strategy := &infrav1.BinaryUpgradeStrategy{
		Type:          "Rolling",
		MaxConcurrent: 1,
		Timeout:       &metav1.Duration{Duration: 600000000000},
		RetryCount:    3,
		Drain:         true,
	}

	if strategy.Type != "Rolling" {
		t.Errorf("expected type Rolling, got %s", strategy.Type)
	}
	if strategy.MaxConcurrent != 1 {
		t.Errorf("expected max concurrent 1, got %d", strategy.MaxConcurrent)
	}
	if !strategy.Drain {
		t.Errorf("expected drain true")
	}
}

func TestBinaryComponentWithAllFields(t *testing.T) {
	component := infrav1.BinaryComponent{
		Version:       "1.7.24",
		Type:          infrav1.ComponentTypeBinary,
		Path:          "/opt/capbm/binaries/containerd",
		Architectures: []string{"amd64", "arm64"},
		Files: infrav1.BinaryFiles{
			Archive: "containerd-1.7.24.tar.gz",
		},
		InstallStrategy: &infrav1.BinaryInstallStrategy{
			Timeout:     &metav1.Duration{Duration: 300000000000},
			RetryCount:  3,
			Method:      "package",
			ServiceName: "containerd",
		},
		UpgradeStrategy: &infrav1.BinaryUpgradeStrategy{
			Type:          "Rolling",
			MaxConcurrent: 1,
			Timeout:       &metav1.Duration{Duration: 600000000000},
			RetryCount:    3,
			Drain:         true,
		},
		PreHooks: []infrav1.AddonHook{
			{
				Name:      "stop-containerd",
				Command:   "systemctl stop containerd",
				Timeout:   &metav1.Duration{Duration: 30000000000},
				OnFailure: "Abort",
			},
		},
		PostHooks: []infrav1.AddonHook{
			{
				Name:      "start-containerd",
				Command:   "systemctl start containerd",
				Timeout:   &metav1.Duration{Duration: 30000000000},
				OnFailure: "Abort",
			},
		},
		Upgrade: &infrav1.ComponentUpgradeConfig{
			Backup: infrav1.ComponentBackupConfig{
				Enabled: true,
				Config: []infrav1.BackupItem{
					{Path: "/etc/containerd/config.toml", Type: "file"},
				},
			},
			Rollback: infrav1.ComponentRollbackConfig{
				Script:  "scripts/rollback-containerd.sh",
				Timeout: &metav1.Duration{Duration: 300000000000},
			},
			HealthCheck: infrav1.ComponentHealthCheckConfig{
				Command: "systemctl is-active containerd",
				Timeout: &metav1.Duration{Duration: 30000000000},
				Retries: 3,
			},
		},
	}

	if component.Version != "1.7.24" {
		t.Errorf("expected version 1.7.24, got %s", component.Version)
	}
	if component.InstallStrategy == nil {
		t.Errorf("expected InstallStrategy to be set")
	}
	if component.UpgradeStrategy == nil {
		t.Errorf("expected UpgradeStrategy to be set")
	}
	if len(component.PreHooks) != 1 {
		t.Errorf("expected 1 pre-hook, got %d", len(component.PreHooks))
	}
	if len(component.PostHooks) != 1 {
		t.Errorf("expected 1 post-hook, got %d", len(component.PostHooks))
	}
	if component.Upgrade == nil {
		t.Errorf("expected Upgrade to be set")
	}
}

func TestKubernetesComponentWithAllFields(t *testing.T) {
	component := infrav1.KubernetesComponent{
		Version: "v1.31.0",
		Type:    infrav1.ComponentTypeBinary,
		Path:    "/opt/capbm/binaries/kubernetes",
		Platforms: map[string]infrav1.K8SPlatform{
			"ubuntu": {
				Architectures: []string{"amd64", "arm64"},
				Packages: map[string]string{
					"kubeadm": "kubeadm_1.31.0-00",
					"kubelet": "kubelet_1.31.0-00",
					"kubectl": "kubectl_1.31.0-00",
				},
			},
		},
		InstallStrategy: &infrav1.BinaryInstallStrategy{
			Timeout:     &metav1.Duration{Duration: 600000000000},
			RetryCount:  3,
			Method:      "package",
			ServiceName: "kubelet",
		},
		UpgradeStrategy: &infrav1.BinaryUpgradeStrategy{
			Type:          "Rolling",
			MaxConcurrent: 1,
			Timeout:       &metav1.Duration{Duration: 900000000000},
			RetryCount:    3,
			Drain:         true,
		},
		PreHooks: []infrav1.AddonHook{
			{
				Name:      "drain-node",
				Command:   "kubectl drain {{.NodeName}} --ignore-daemonsets --delete-emptydir-data",
				Timeout:   &metav1.Duration{Duration: 300000000000},
				OnFailure: "Abort",
			},
		},
		PostHooks: []infrav1.AddonHook{
			{
				Name:      "uncordon-node",
				Command:   "kubectl uncordon {{.NodeName}}",
				Timeout:   &metav1.Duration{Duration: 30000000000},
				OnFailure: "Abort",
			},
		},
		Upgrade: &infrav1.ComponentUpgradeConfig{
			Backup: infrav1.ComponentBackupConfig{
				Enabled:      true,
				EtcdSnapshot: true,
				Config: []infrav1.BackupItem{
					{Path: "/etc/kubernetes", Type: "directory"},
				},
			},
			Rollback: infrav1.ComponentRollbackConfig{
				Script:  "scripts/rollback-kubernetes.sh",
				Timeout: &metav1.Duration{Duration: 600000000000},
			},
			HealthCheck: infrav1.ComponentHealthCheckConfig{
				Command: "kubectl get nodes {{.NodeName}} -o jsonpath='{.status.conditions[?(@.type==\"Ready\")].status}'",
				Timeout: &metav1.Duration{Duration: 60000000000},
				Retries: 3,
			},
		},
	}

	if component.Version != "v1.31.0" {
		t.Errorf("expected version v1.31.0, got %s", component.Version)
	}
	if component.InstallStrategy == nil {
		t.Errorf("expected InstallStrategy to be set")
	}
	if component.UpgradeStrategy == nil {
		t.Errorf("expected UpgradeStrategy to be set")
	}
	if len(component.PreHooks) != 1 {
		t.Errorf("expected 1 pre-hook, got %d", len(component.PreHooks))
	}
	if len(component.PostHooks) != 1 {
		t.Errorf("expected 1 post-hook, got %d", len(component.PostHooks))
	}
	if component.Upgrade == nil {
		t.Errorf("expected Upgrade to be set")
	}
	if !component.Upgrade.Backup.EtcdSnapshot {
		t.Errorf("expected etcdSnapshot true")
	}
}

func TestBinaryUpgradeStrategyTypes(t *testing.T) {
	tests := []struct {
		name string
		typ  string
	}{
		{"rolling", "Rolling"},
		{"drain-and-upgrade", "DrainAndUpgrade"},
		{"parallel", "Parallel"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strategy := infrav1.BinaryUpgradeStrategy{
				Type: tt.typ,
			}
			if strategy.Type != tt.typ {
				t.Errorf("expected type %s, got %s", tt.typ, strategy.Type)
			}
		})
	}
}

func TestBinaryInstallStrategyMethods(t *testing.T) {
	tests := []struct {
		name   string
		method string
	}{
		{"package", "package"},
		{"archive", "archive"},
		{"manual", "manual"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strategy := infrav1.BinaryInstallStrategy{
				Method: tt.method,
			}
			if strategy.Method != tt.method {
				t.Errorf("expected method %s, got %s", tt.method, strategy.Method)
			}
		})
	}
}

func TestReleaseImageWithBinaryComponents(t *testing.T) {
	releaseImage := infrav1.ReleaseImage{
		Spec: infrav1.ReleaseImageSpec{
			Version: "v1.31.0",
			Image:   "registry.example.com/capbm/release:v1.31.0",
			Components: infrav1.ReleaseComponentVersions{
				Kubernetes: infrav1.KubernetesComponent{
					Version: "v1.31.0",
					Type:    infrav1.ComponentTypeBinary,
					InstallStrategy: &infrav1.BinaryInstallStrategy{
						Timeout:     &metav1.Duration{Duration: 600000000000},
						RetryCount:  3,
						Method:      "package",
						ServiceName: "kubelet",
					},
					UpgradeStrategy: &infrav1.BinaryUpgradeStrategy{
						Type:          "Rolling",
						MaxConcurrent: 1,
						Timeout:       &metav1.Duration{Duration: 900000000000},
						RetryCount:    3,
						Drain:         true,
					},
					PreHooks: []infrav1.AddonHook{
						{Name: "drain-node", Command: "kubectl drain {{.NodeName}}", OnFailure: "Abort"},
					},
					PostHooks: []infrav1.AddonHook{
						{Name: "uncordon-node", Command: "kubectl uncordon {{.NodeName}}", OnFailure: "Abort"},
					},
				},
				Containerd: infrav1.BinaryComponent{
					Version: "1.7.24",
					Type:    infrav1.ComponentTypeBinary,
					InstallStrategy: &infrav1.BinaryInstallStrategy{
						Timeout:     &metav1.Duration{Duration: 300000000000},
						RetryCount:  3,
						Method:      "package",
						ServiceName: "containerd",
					},
					UpgradeStrategy: &infrav1.BinaryUpgradeStrategy{
						Type:          "Rolling",
						MaxConcurrent: 1,
						Timeout:       &metav1.Duration{Duration: 600000000000},
						RetryCount:    3,
						Drain:         true,
					},
					PreHooks: []infrav1.AddonHook{
						{Name: "stop-containerd", Command: "systemctl stop containerd", OnFailure: "Abort"},
					},
					PostHooks: []infrav1.AddonHook{
						{Name: "start-containerd", Command: "systemctl start containerd", OnFailure: "Abort"},
					},
				},
			},
		},
	}

	// Verify Kubernetes component
	if releaseImage.Spec.Components.Kubernetes.InstallStrategy == nil {
		t.Errorf("expected Kubernetes InstallStrategy to be set")
	}
	if releaseImage.Spec.Components.Kubernetes.UpgradeStrategy == nil {
		t.Errorf("expected Kubernetes UpgradeStrategy to be set")
	}
	if len(releaseImage.Spec.Components.Kubernetes.PreHooks) != 1 {
		t.Errorf("expected 1 Kubernetes pre-hook, got %d", len(releaseImage.Spec.Components.Kubernetes.PreHooks))
	}

	// Verify containerd component
	if releaseImage.Spec.Components.Containerd.InstallStrategy == nil {
		t.Errorf("expected Containerd InstallStrategy to be set")
	}
	if releaseImage.Spec.Components.Containerd.UpgradeStrategy == nil {
		t.Errorf("expected Containerd UpgradeStrategy to be set")
	}
	if len(releaseImage.Spec.Components.Containerd.PostHooks) != 1 {
		t.Errorf("expected 1 Containerd post-hook, got %d", len(releaseImage.Spec.Components.Containerd.PostHooks))
	}
}
