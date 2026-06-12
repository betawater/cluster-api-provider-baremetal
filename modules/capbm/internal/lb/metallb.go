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

package lb

import (
	"context"
	"fmt"
	"net"
	"time"

	capbmv1 "github.com/BetaWater/cluster-api-provider-baremetal/modules/capbm/api/v1beta2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MetalLBProvider implements the Provider interface for MetalLB.
type MetalLBProvider struct {
	config    *capbmv1.MetalLBConfig
	k8sClient client.Client
	namespace string
	backends  []Backend
}

// NewMetalLBProvider creates a new MetalLB provider.
func NewMetalLBProvider(config *capbmv1.MetalLBConfig) (Provider, error) {
	if config == nil {
		return nil, fmt.Errorf("MetalLB configuration is required")
	}
	return &MetalLBProvider{
		config:   config,
		backends: make([]Backend, 0),
	}, nil
}

// WithK8sClient sets the Kubernetes client for MetalLB operations.
func (p *MetalLBProvider) WithK8sClient(c client.Client, namespace string) *MetalLBProvider {
	p.k8sClient = c
	p.namespace = namespace
	return p
}

// RegisterBackend adds a backend server by updating the MetalLB IPAddressPool and Service.
func (p *MetalLBProvider) RegisterBackend(ctx context.Context, backend Backend) error {
	// Check if backend already exists
	for _, b := range p.backends {
		if b.Name == backend.Name && b.IP == backend.IP {
			return nil
		}
	}

	p.backends = append(p.backends, backend)

	// If Kubernetes client is available, update MetalLB resources
	if p.k8sClient != nil {
		if err := p.updateService(ctx, backend); err != nil {
			return fmt.Errorf("failed to update Service: %w", err)
		}
	}

	return nil
}

// UnregisterBackend removes a backend server.
func (p *MetalLBProvider) UnregisterBackend(ctx context.Context, backend Backend) error {
	for i, b := range p.backends {
		if b.Name == backend.Name && b.IP == backend.IP {
			p.backends = append(p.backends[:i], p.backends[i+1:]...)
			break
		}
	}

	return nil
}

// GetBackends returns the current list of backend servers.
func (p *MetalLBProvider) GetBackends(ctx context.Context) ([]Backend, error) {
	// If Kubernetes client is available, get backends from Services
	if p.k8sClient != nil {
		return p.getBackendsFromServices(ctx)
	}
	return p.backends, nil
}

// HealthCheck checks if a backend is healthy.
func (p *MetalLBProvider) HealthCheck(ctx context.Context, backend Backend) (bool, error) {
	addr := net.JoinHostPort(backend.IP, fmt.Sprintf("%d", backend.Port))
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return false, nil
	}
	if err := conn.Close(); err != nil {
		return false, fmt.Errorf("failed to close connection: %w", err)
	}
	return true, nil
}

// updateService updates or creates a Kubernetes Service with LoadBalancer type.
func (p *MetalLBProvider) updateService(ctx context.Context, backend Backend) error {
	serviceName := fmt.Sprintf("svc-%s", backend.Name)

	svc := &corev1.Service{}
	err := p.k8sClient.Get(ctx, types.NamespacedName{
		Name:      serviceName,
		Namespace: p.namespace,
	}, svc)

	if err != nil {
		// Create new Service
		svc = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: p.namespace,
				Labels: map[string]string{
					"app.kubernetes.io/part-of": "capbm",
					"app":                       backend.Name,
				},
				Annotations: map[string]string{
					"metallb.universe.tf/address-pool": p.config.AddressPool,
				},
			},
			Spec: corev1.ServiceSpec{
				Type: corev1.ServiceTypeLoadBalancer,
				Ports: []corev1.ServicePort{
					{
						Port:       int32(backend.Port),
						TargetPort: intstr.FromInt(backend.Port),
						Protocol:   corev1.ProtocolTCP,
					},
				},
				Selector: map[string]string{
					"app": backend.Name,
				},
			},
		}
		if p.config.LoadBalancerIP != "" {
			svc.Spec.LoadBalancerIP = p.config.LoadBalancerIP
		}
		return p.k8sClient.Create(ctx, svc)
	}

	// Update existing Service
	original := svc.DeepCopy()
	if svc.Annotations == nil {
		svc.Annotations = make(map[string]string)
	}
	svc.Annotations["metallb.universe.tf/address-pool"] = p.config.AddressPool
	svc.Spec.Ports = []corev1.ServicePort{
		{
			Port:       int32(backend.Port),
			TargetPort: intstr.FromInt(backend.Port),
			Protocol:   corev1.ProtocolTCP,
		},
	}
	svc.Spec.Selector = map[string]string{
		"app": backend.Name,
	}
	if p.config.LoadBalancerIP != "" {
		svc.Spec.LoadBalancerIP = p.config.LoadBalancerIP
	}
	return p.k8sClient.Patch(ctx, svc, client.MergeFrom(original))
}

// getBackendsFromServices retrieves backends from MetalLB-managed Services.
func (p *MetalLBProvider) getBackendsFromServices(ctx context.Context) ([]Backend, error) {
	svcList := &corev1.ServiceList{}
	if err := p.k8sClient.List(ctx, svcList, client.InNamespace(p.namespace), client.MatchingLabels{
		"app.kubernetes.io/part-of": "capbm",
	}); err != nil {
		return nil, err
	}

	var backends []Backend
	for _, svc := range svcList.Items {
		if svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
			continue
		}

		for _, ingress := range svc.Status.LoadBalancer.Ingress {
			for _, port := range svc.Spec.Ports {
				backends = append(backends, Backend{
					Name: svc.Name,
					IP:   ingress.IP,
					Port: int(port.Port),
				})
			}
		}
	}

	return backends, nil
}
