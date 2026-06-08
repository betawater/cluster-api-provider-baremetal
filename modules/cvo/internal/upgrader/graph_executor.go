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
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	cfov1 "github.com/BetaWater/cluster-api-provider-baremetal/modules/cvo/api/v1beta1"
	capbmssh "github.com/BetaWater/cluster-api-provider-baremetal/modules/cvo/pkg/ssh"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type GraphExecutor struct {
	client        client.Client
	puller        *OCIPuller
	healthChecker *HealthChecker
	sshManager    *capbmssh.SSHManager
}

func NewGraphExecutor(c client.Client, puller *OCIPuller, healthChecker *HealthChecker) *GraphExecutor {
	return &GraphExecutor{client: c, puller: puller, healthChecker: healthChecker}
}

// WithSSHManager sets the SSH manager for script execution.
func (e *GraphExecutor) WithSSHManager(m *capbmssh.SSHManager) *GraphExecutor {
	e.sshManager = m
	return e
}

func (e *GraphExecutor) ValidateUpgradePath(ctx context.Context, cv *cfov1.ClusterVersion) error {
	if cv.Spec.DesiredUpdate == nil || cv.Spec.DesiredUpdate.Version == "" {
		return nil
	}

	upgradePath := &cfov1.UpgradePath{}
	if err := e.client.Get(ctx, types.NamespacedName{Name: "global"}, upgradePath); err != nil {
		return fmt.Errorf("failed to get UpgradePath: %w", err)
	}

	from := cv.Status.ActualVersion
	to := cv.Spec.DesiredUpdate.Version

	if from == to {
		return nil
	}

	found := false
	for _, edge := range upgradePath.Spec.Graph.Edges {
		if matchVersion(edge.From, from) && matchVersion(edge.To, to) {
			found = true
			if !edge.Recommended && !cv.Spec.DesiredUpdate.Force {
				return fmt.Errorf("upgrade from %s to %s is not recommended, use force=true to override", from, to)
			}
			break
		}
	}

	if !found && !cv.Spec.DesiredUpdate.Force {
		return fmt.Errorf("no valid upgrade path from %s to %s", from, to)
	}

	for _, blocked := range upgradePath.Spec.Rules.BlockedUpgrades {
		if matchVersion(blocked.From, from) && matchVersion(blocked.To, to) {
			return fmt.Errorf("upgrade from %s to %s is blocked: %s", from, to, blocked.Reason)
		}
	}

	return nil
}

func (e *GraphExecutor) ComputeAvailableUpdates(ctx context.Context, cv *cfov1.ClusterVersion) ([]cfov1.Release, error) {
	upgradePath := &cfov1.UpgradePath{}
	if err := e.client.Get(ctx, types.NamespacedName{Name: "global"}, upgradePath); err != nil {
		return nil, err
	}

	releaseCatalog := &cfov1.ReleaseCatalog{}
	if err := e.client.Get(ctx, types.NamespacedName{Name: "global"}, releaseCatalog); err != nil {
		return nil, err
	}

	from := cv.Status.ActualVersion
	var updates []cfov1.Release

	for _, edge := range upgradePath.Spec.Graph.Edges {
		if matchVersion(edge.From, from) {
			for _, entry := range releaseCatalog.Status.Releases {
				if matchVersion(edge.To, entry.Version) {
					updates = append(updates, cfov1.Release{
						Version: entry.Version,
						Image:   entry.Image,
					})
					break
				}
			}
		}
	}

	sort.Slice(updates, func(i, j int) bool {
		return updates[i].Version < updates[j].Version
	})

	return updates, nil
}

func (e *GraphExecutor) ExecuteUpgradeGraph(ctx context.Context, cv *cfov1.ClusterVersion, releaseImage *cfov1.ReleaseImage) error {
	phases := releaseImage.Spec.UpgradeGraph
	sort.Slice(phases, func(i, j int) bool {
		return phases[i].Order < phases[j].Order
	})

	completed := make(map[string]bool)

	for _, phase := range phases {
		if err := e.executePhase(ctx, phase, releaseImage, completed); err != nil {
			if phase.Blocking {
				return fmt.Errorf("phase %s failed: %w", phase.Name, err)
			}
		}
		for _, comp := range phase.Components {
			completed[comp.Name] = true
		}
	}

	return nil
}

func (e *GraphExecutor) executePhase(ctx context.Context, phase cfov1.UpgradePhase, releaseImage *cfov1.ReleaseImage, completed map[string]bool) error {
	depGraph := buildDependencyGraph(phase.Components)
	sorted := topologicalSort(depGraph)

	for _, compName := range sorted {
		comp := findComponent(phase.Components, compName)
		if comp == nil {
			continue
		}

		for _, dep := range comp.DependsOn {
			if !completed[dep] {
				return fmt.Errorf("dependency %s not completed for component %s", dep, comp.Name)
			}
		}

		if err := e.executeComponent(ctx, *comp, releaseImage); err != nil {
			if comp.Blocking {
				return fmt.Errorf("component %s failed: %w", comp.Name, err)
			}
		}
	}

	return nil
}

func (e *GraphExecutor) executeComponent(ctx context.Context, comp cfov1.UpgradeComponent, releaseImage *cfov1.ReleaseImage) error {
	if len(comp.Manifests) > 0 {
		if err := e.applyManifests(ctx, comp.Manifests, releaseImage); err != nil {
			return err
		}
	}
	if len(comp.Scripts) > 0 {
		if err := e.executeScripts(ctx, comp.Scripts, releaseImage); err != nil {
			return err
		}
	}
	if comp.HealthCheck != nil && e.healthChecker != nil {
		if err := e.runHealthCheck(ctx, comp.HealthCheck); err != nil {
			return fmt.Errorf("health check failed for %s: %w", comp.Name, err)
		}
	}
	return nil
}

func (e *GraphExecutor) applyManifests(ctx context.Context, manifests []string, releaseImage *cfov1.ReleaseImage) error {
	manifestDir, err := e.puller.GetManifestDir(ctx, releaseImage.Spec.Image)
	if err != nil {
		return fmt.Errorf("failed to get manifest dir: %w", err)
	}

	for _, manifest := range manifests {
		path := filepath.Join(manifestDir, manifest)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return fmt.Errorf("manifest file not found: %s", path)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read manifest %s: %w", path, err)
		}

		obj, err := decodeYAML(data)
		if err != nil {
			return fmt.Errorf("failed to decode manifest %s: %w", path, err)
		}

		//nolint:staticcheck // client.Apply deprecated, will migrate to Server-Side Apply when ready
		if err := e.client.Patch(ctx, obj, client.Apply, client.ForceOwnership, client.FieldOwner("capbm-upgrader")); err != nil {
			return fmt.Errorf("failed to apply manifest %s: %w", path, err)
		}
	}

	return nil
}

func (e *GraphExecutor) executeScripts(ctx context.Context, scripts []string, releaseImage *cfov1.ReleaseImage) error {
	scriptsDir, err := e.puller.GetScriptsDir(ctx, releaseImage.Spec.Image)
	if err != nil {
		return fmt.Errorf("failed to get scripts dir: %w", err)
	}

	for _, script := range scripts {
		path := filepath.Join(scriptsDir, script)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return fmt.Errorf("script file not found: %s", path)
		}

		scriptContent, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read script %s: %w", path, err)
		}

		// Execute script via SSH if SSH manager is available
		if e.sshManager != nil {
			// Get target nodes from release image
			nodes, err := e.getTargetNodes(ctx, releaseImage)
			if err != nil {
				return fmt.Errorf("failed to get target nodes: %w", err)
			}

			for _, node := range nodes {
				sshConn, err := e.sshManager.Connect(node.Name, 22, capbmssh.Credentials{})
				if err != nil {
					return fmt.Errorf("failed to connect to node %s: %w", node.Name, err)
				}

				result, execErr := sshConn.ExecuteScript(ctx, string(scriptContent))
				e.sshManager.Close(sshConn)

				if execErr != nil {
					return fmt.Errorf("script execution failed on node %s: %w", node.Name, execErr)
				}
				if result.ExitCode != 0 {
					return fmt.Errorf("script failed on node %s with exit code %d: %s", node.Name, result.ExitCode, result.Stderr)
				}
			}
		}
	}

	return nil
}

// getTargetNodes returns the list of nodes to execute scripts on.
func (e *GraphExecutor) getTargetNodes(ctx context.Context, releaseImage *cfov1.ReleaseImage) ([]*corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	if err := e.client.List(ctx, nodeList); err != nil {
		return nil, err
	}

	var nodes []*corev1.Node
	for i := range nodeList.Items {
		nodes = append(nodes, &nodeList.Items[i])
	}
	return nodes, nil
}

func (e *GraphExecutor) runHealthCheck(ctx context.Context, hc *cfov1.HealthCheck) error {
	if e.healthChecker == nil {
		return nil
	}

	timeout := hc.Timeout.Duration
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	switch hc.Type {
	case "PodReady":
		return e.healthChecker.WaitForPodsReady(ctx, hc.Namespace, hc.LabelSelector, timeout)
	case "DaemonSetReady":
		return e.healthChecker.WaitForDaemonSetReady(ctx, hc.Namespace, hc.Name, timeout)
	case "DeploymentReady":
		return e.healthChecker.WaitForDeploymentReady(ctx, hc.Namespace, hc.Name, timeout)
	case "EndpointHealthy":
		return e.waitForEndpointHealthy(ctx, hc.Endpoint, timeout)
	case "CRDEstablished":
		return e.waitForCRDEstablished(ctx, hc.Name, timeout)
	case "ServiceRunning":
		return e.waitForServiceRunning(ctx, hc.Name, timeout)
	default:
		return fmt.Errorf("unknown health check type: %s", hc.Type)
	}
}

func (e *GraphExecutor) waitForEndpointHealthy(ctx context.Context, endpoint string, timeout time.Duration) error {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	return wait.PollUntilContextTimeout(ctx, 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		resp, err := client.Get(endpoint)
		if err != nil {
			return false, nil
		}
		defer resp.Body.Close()

		return resp.StatusCode >= 200 && resp.StatusCode < 300, nil
	})
}

func (e *GraphExecutor) waitForCRDEstablished(ctx context.Context, name string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(ctx, 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		crd := &apiextensionsv1.CustomResourceDefinition{}
		if err := e.client.Get(ctx, types.NamespacedName{Name: name}, crd); err != nil {
			return false, nil
		}
		for _, cond := range crd.Status.Conditions {
			if cond.Type == apiextensionsv1.Established && cond.Status == apiextensionsv1.ConditionTrue {
				return true, nil
			}
		}
		return false, nil
	})
}

func (e *GraphExecutor) waitForServiceRunning(ctx context.Context, name string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(ctx, 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		podList := &corev1.PodList{}
		if err := e.client.List(ctx, podList, client.MatchingLabels{"app": name}); err != nil {
			return false, nil
		}
		for _, pod := range podList.Items {
			if pod.Status.Phase != corev1.PodRunning {
				return false, nil
			}
		}
		return len(podList.Items) > 0, nil
	})
}

type depNode struct {
	name string
	deps []string
}

func buildDependencyGraph(components []cfov1.UpgradeComponent) map[string]*depNode {
	graph := make(map[string]*depNode)
	for _, comp := range components {
		graph[comp.Name] = &depNode{name: comp.Name, deps: comp.DependsOn}
	}
	return graph
}

func topologicalSort(graph map[string]*depNode) []string {
	var result []string
	visited := make(map[string]bool)
	inProgress := make(map[string]bool)

	var visit func(name string)
	visit = func(name string) {
		if visited[name] {
			return
		}
		if inProgress[name] {
			return
		}
		inProgress[name] = true
		node := graph[name]
		if node != nil {
			for _, dep := range node.deps {
				visit(dep)
			}
		}
		inProgress[name] = false
		visited[name] = true
		result = append(result, name)
	}

	for name := range graph {
		visit(name)
	}

	return result
}

func findComponent(components []cfov1.UpgradeComponent, name string) *cfov1.UpgradeComponent {
	for i := range components {
		if components[i].Name == name {
			return &components[i]
		}
	}
	return nil
}

func matchVersion(pattern, version string) bool {
	if pattern == "" || version == "" {
		return false
	}
	if len(pattern) > 0 && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return len(version) >= len(prefix) && version[:len(prefix)] == prefix
	}
	return pattern == version
}

func decodeYAML(data []byte) (client.Object, error) {
	// Use the universal decoder to handle any Kubernetes object
	decode := scheme.Codecs.UniversalDeserializer().Decode

	obj, _, err := decode(data, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decode YAML: %w", err)
	}

	// Convert to client.Object
	clientObj, ok := obj.(client.Object)
	if !ok {
		return nil, fmt.Errorf("decoded object does not implement client.Object")
	}

	return clientObj, nil
}
