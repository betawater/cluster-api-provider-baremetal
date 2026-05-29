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
	"sort"

	infrav1 "github.com/BetaWater/cluster-api-provider-baremetal/api/v1beta1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type GraphExecutor struct {
	client client.Client
	puller *OCIPuller
}

func NewGraphExecutor(c client.Client, puller *OCIPuller) *GraphExecutor {
	return &GraphExecutor{client: c, puller: puller}
}

func (e *GraphExecutor) ValidateUpgradePath(ctx context.Context, cv *infrav1.ClusterVersion) error {
	if cv.Spec.DesiredUpdate == nil || cv.Spec.DesiredUpdate.Version == "" {
		return nil
	}

	upgradePath := &infrav1.UpgradePath{}
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

func (e *GraphExecutor) ComputeAvailableUpdates(ctx context.Context, cv *infrav1.ClusterVersion) ([]infrav1.Release, error) {
	upgradePath := &infrav1.UpgradePath{}
	if err := e.client.Get(ctx, types.NamespacedName{Name: "global"}, upgradePath); err != nil {
		return nil, err
	}

	releaseCatalog := &infrav1.ReleaseCatalog{}
	if err := e.client.Get(ctx, types.NamespacedName{Name: "global"}, releaseCatalog); err != nil {
		return nil, err
	}

	from := cv.Status.ActualVersion
	var updates []infrav1.Release

	for _, edge := range upgradePath.Spec.Graph.Edges {
		if matchVersion(edge.From, from) {
			for _, entry := range releaseCatalog.Status.Releases {
				if matchVersion(edge.To, entry.Version) {
					updates = append(updates, infrav1.Release{
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

func (e *GraphExecutor) ExecuteUpgradeGraph(ctx context.Context, cv *infrav1.ClusterVersion, releaseImage *infrav1.ReleaseImage) error {
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

func (e *GraphExecutor) executePhase(ctx context.Context, phase infrav1.UpgradePhase, releaseImage *infrav1.ReleaseImage, completed map[string]bool) error {
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

func (e *GraphExecutor) executeComponent(ctx context.Context, comp infrav1.UpgradeComponent, releaseImage *infrav1.ReleaseImage) error {
	if len(comp.Manifests) > 0 {
		return e.applyManifests(ctx, comp.Manifests, releaseImage)
	}
	if len(comp.Scripts) > 0 {
		return e.executeScripts(ctx, comp.Scripts, releaseImage)
	}
	return nil
}

func (e *GraphExecutor) applyManifests(ctx context.Context, manifests []string, releaseImage *infrav1.ReleaseImage) error {
	_ = ctx
	_ = manifests
	_ = releaseImage
	return nil
}

func (e *GraphExecutor) executeScripts(ctx context.Context, scripts []string, releaseImage *infrav1.ReleaseImage) error {
	_ = ctx
	_ = scripts
	_ = releaseImage
	return nil
}

type depNode struct {
	name string
	deps []string
}

func buildDependencyGraph(components []infrav1.UpgradeComponent) map[string]*depNode {
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

func findComponent(components []infrav1.UpgradeComponent, name string) *infrav1.UpgradeComponent {
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
