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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta2"

	infrav1 "github.com/BetaWater/cluster-api-provider-baremetal/api/v1beta2"
)

var _ = Describe("ClusterClass E2E Tests", func() {
	Context("When creating a cluster with ClusterClass", func() {
		It("Should create cluster infrastructure successfully", func() {
			ctx := context.Background()
			namespace := "default"
			clusterName := "test-cluster"

			By("Creating SSH credentials Secret")
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ssh-credentials",
					Namespace: namespace,
				},
				StringData: map[string]string{
					"username": "root",
					"password": "testpassword",
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())

			By("Creating BareMetalCluster directly")
			bmCluster := &infrav1.BareMetalCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clusterName,
					Namespace: namespace,
				},
				Spec: infrav1.BareMetalClusterSpec{
					ControlPlaneEndpoint: clusterv1.APIEndpoint{
						Host: "lb.example.com",
						Port: 6443,
					},
				},
			}
			Expect(k8sClient.Create(ctx, bmCluster)).Should(Succeed())

			By("Waiting for BareMetalCluster to be ready")
			Eventually(func() bool {
				key := types.NamespacedName{Name: clusterName, Namespace: namespace}
				cluster := &infrav1.BareMetalCluster{}
				if err := k8sClient.Get(ctx, key, cluster); err != nil {
					return false
				}
				return cluster.Status.Ready
			}, 30*time.Second, 1*time.Second).Should(BeTrue())

			By("Cleaning up")
			Expect(k8sClient.Delete(ctx, bmCluster)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, secret)).Should(Succeed())
		})
	})
})
