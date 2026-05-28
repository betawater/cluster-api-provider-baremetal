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

package e2e

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrav1 "github.com/BetaWater/cluster-api-provider-baremetal/api/v1beta1"
)

// CreateSSHSecret creates an SSH credentials Secret for testing.
func CreateSSHSecret(ctx context.Context, k8sClient client.Client, name, namespace, username, password string) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		StringData: map[string]string{
			"username": username,
			"password": password,
		},
	}
	return k8sClient.Create(ctx, secret)
}

// CreateBareMetalCluster creates a BareMetalCluster for testing.
func CreateBareMetalCluster(ctx context.Context, k8sClient client.Client, name, namespace, endpointHost string, endpointPort int) error {
	cluster := &infrav1.BareMetalCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: infrav1.BareMetalClusterSpec{
			ControlPlaneEndpoint: infrav1.APIEndpoint{
				Host: endpointHost,
				Port: int32(endpointPort),
			},
		},
	}
	return k8sClient.Create(ctx, cluster)
}

// WaitForClusterReady waits for a BareMetalCluster to become ready.
func WaitForClusterReady(ctx context.Context, k8sClient client.Client, name, namespace string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		key := types.NamespacedName{Name: name, Namespace: namespace}
		cluster := &infrav1.BareMetalCluster{}
		if err := k8sClient.Get(ctx, key, cluster); err != nil {
			if apierrors.IsNotFound(err) {
				time.Sleep(1 * time.Second)
				continue
			}
			return err
		}
		if cluster.Status.Ready {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("timeout waiting for cluster %s/%s to be ready", namespace, name)
}

// CleanupCluster deletes a BareMetalCluster and waits for it to be removed.
func CleanupCluster(ctx context.Context, k8sClient client.Client, name, namespace string, timeout time.Duration) error {
	key := types.NamespacedName{Name: name, Namespace: namespace}
	cluster := &infrav1.BareMetalCluster{}
	if err := k8sClient.Get(ctx, key, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if err := k8sClient.Delete(ctx, cluster); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := k8sClient.Get(ctx, key, cluster); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("timeout waiting for cluster %s/%s to be deleted", namespace, name)
}
