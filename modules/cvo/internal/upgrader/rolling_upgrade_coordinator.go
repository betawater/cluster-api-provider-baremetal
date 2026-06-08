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
	"sync"

	corev1 "k8s.io/api/core/v1"
)

// RollingUpgradeCoordinator coordinates batched rolling upgrades across nodes.
type RollingUpgradeCoordinator struct {
	maxUnavailable int
	maxConcurrent  int
}

// NewRollingUpgradeCoordinator creates a new rolling upgrade coordinator.
func NewRollingUpgradeCoordinator(maxUnavailable, maxConcurrent int) *RollingUpgradeCoordinator {
	if maxUnavailable <= 0 {
		maxUnavailable = 1
	}
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}
	return &RollingUpgradeCoordinator{
		maxUnavailable: maxUnavailable,
		maxConcurrent:  maxConcurrent,
	}
}

// UpgradeFunc is the function signature for upgrading a single node.
type UpgradeFunc func(ctx context.Context, node *corev1.Node) error

// ExecuteRollingUpgrade executes a rolling upgrade across all nodes in batches.
func (c *RollingUpgradeCoordinator) ExecuteRollingUpgrade(ctx context.Context, nodes []*corev1.Node, upgradeFn UpgradeFunc) error {
	if len(nodes) == 0 {
		return nil
	}

	batches := c.createBatches(nodes)

	for i, batch := range batches {
		if err := c.executeBatch(ctx, batch, upgradeFn); err != nil {
			return fmt.Errorf("batch %d failed: %w", i+1, err)
		}
	}

	return nil
}

// createBatches divides nodes into batches based on maxUnavailable.
func (c *RollingUpgradeCoordinator) createBatches(nodes []*corev1.Node) [][]*corev1.Node {
	var batches [][]*corev1.Node

	for i := 0; i < len(nodes); i += c.maxUnavailable {
		end := i + c.maxUnavailable
		if end > len(nodes) {
			end = len(nodes)
		}
		batches = append(batches, nodes[i:end])
	}

	return batches
}

// executeBatch upgrades all nodes in a batch concurrently.
func (c *RollingUpgradeCoordinator) executeBatch(ctx context.Context, batch []*corev1.Node, upgradeFn UpgradeFunc) error {
	var (
		wg      sync.WaitGroup
		errChan = make(chan error, len(batch))
		sem     = make(chan struct{}, c.maxConcurrent)
	)

	for _, node := range batch {
		wg.Add(1)
		go func(n *corev1.Node) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			if err := upgradeFn(ctx, n); err != nil {
				errChan <- fmt.Errorf("failed to upgrade node %s: %w", n.Name, err)
			}
		}(node)
	}

	wg.Wait()
	close(errChan)

	// Collect errors
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("%d node(s) failed to upgrade: %v", len(errs), errs[0])
	}

	return nil
}
