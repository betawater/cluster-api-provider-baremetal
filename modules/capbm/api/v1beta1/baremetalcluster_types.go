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

package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

const (
	// ClusterFinalizer allows ReconcileBareMetalCluster to clean up resources associated with BareMetalCluster before
	// removing it from the apiserver.
	ClusterFinalizer = "baremetalcluster.infrastructure.cluster.x-k8s.io"

	// ClusterReadyCondition reports the current status of the cluster infrastructure.
	ClusterReadyCondition = clusterv1.ReadyCondition

	// EndpointNotSetReason indicates the control plane endpoint is not set.
	EndpointNotSetReason = "EndpointNotSet"

	// EndpointSourceAnnotation indicates the source of the control plane endpoint.
	EndpointSourceAnnotation = "baremetal.cluster.x-k8s.io/endpoint-source"
)

// BareMetalClusterSpec defines the desired state of BareMetalCluster.
type BareMetalClusterSpec struct {
	// ControlPlaneEndpoint represents the endpoint used to communicate with the control plane.
	// +optional
	ControlPlaneEndpoint clusterv1.APIEndpoint `json:"controlPlaneEndpoint,omitempty"`

	// Network holds the cluster-level network configuration.
	// +optional
	Network NetworkConfig `json:"network,omitempty"`

	// LoadBalancer holds the load balancer configuration for the control plane.
	// +optional
	LoadBalancer *LoadBalancerConfig `json:"loadBalancer,omitempty"`

	// IngressLoadBalancer holds the load balancer configuration for ingress traffic.
	// +optional
	IngressLoadBalancer *IngressLoadBalancerConfig `json:"ingressLoadBalancer,omitempty"`

	// GatewayAPI holds the Gateway API component configuration.
	// +optional
	GatewayAPI *GatewayAPIConfig `json:"gatewayAPI,omitempty"`
}

// NetworkConfig holds the network configuration for the cluster.
type NetworkConfig struct {
	// PodCIDR is the CIDR block for pods.
	// +optional
	PodCIDR string `json:"podCIDR,omitempty"`

	// ServiceCIDR is the CIDR block for services.
	// +optional
	ServiceCIDR string `json:"serviceCIDR,omitempty"`

	// DNSDomain is the DNS domain for the cluster.
	// +optional
	DNSDomain string `json:"dnsDomain,omitempty"`
}

// LoadBalancerConfig defines the load balancer configuration for the control plane.
type LoadBalancerConfig struct {
	// Provider is the load balancer type (haproxy, keepalived, f5, nginx, metal-lb).
	// +optional
	// +kubebuilder:default=haproxy
	Provider string `json:"provider,omitempty"`

	// HealthCheck defines health check configuration.
	// +optional
	HealthCheck HealthCheckConfig `json:"healthCheck,omitempty"`

	// HAProxy holds HAProxy specific configuration.
	// +optional
	HAProxy *HAProxyConfig `json:"haproxy,omitempty"`

	// Keepalived holds Keepalived specific configuration.
	// +optional
	Keepalived *KeepalivedConfig `json:"keepalived,omitempty"`

	// F5 holds F5 BIG-IP specific configuration.
	// +optional
	F5 *F5Config `json:"f5,omitempty"`

	// MetalLB holds MetalLB specific configuration.
	// +optional
	MetalLB *MetalLBConfig `json:"metal-lb,omitempty"`
}

// HealthCheckConfig defines health check configuration for the load balancer.
type HealthCheckConfig struct {
	// Enabled enables health checking.
	// +optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// Path is the health check endpoint.
	// +optional
	// +kubebuilder:default=/healthz
	Path string `json:"path,omitempty"`

	// Interval is the check interval.
	// +optional
	// +kubebuilder:default="5s"
	Interval string `json:"interval,omitempty"`

	// Timeout is the check timeout.
	// +optional
	// +kubebuilder:default="3s"
	Timeout string `json:"timeout,omitempty"`

	// HealthyThreshold is the number of successful checks to mark as healthy.
	// +optional
	// +kubebuilder:default=2
	HealthyThreshold int `json:"healthyThreshold,omitempty"`

	// UnhealthyThreshold is the number of failed checks to mark as unhealthy.
	// +optional
	// +kubebuilder:default=3
	UnhealthyThreshold int `json:"unhealthyThreshold,omitempty"`
}

// HAProxyConfig defines HAProxy specific configuration.
type HAProxyConfig struct {
	// AdminHost is the HAProxy Runtime API host.
	// +optional
	AdminHost string `json:"adminHost,omitempty"`

	// AdminPort is the HAProxy Runtime API port.
	// +optional
	// +kubebuilder:default=9999
	AdminPort int `json:"adminPort,omitempty"`

	// SSHHost is the HAProxy server SSH host (alternative to Runtime API).
	// +optional
	SSHHost string `json:"sshHost,omitempty"`

	// SSHPort is the HAProxy server SSH port.
	// +optional
	// +kubebuilder:default=22
	SSHPort int `json:"sshPort,omitempty"`

	// SSHCredentialsRef references the SSH credentials secret.
	// +optional
	SSHCredentialsRef *corev1.LocalObjectReference `json:"sshCredentialsRef,omitempty"`

	// ConfigFile is the HAProxy configuration file path.
	// +optional
	// +kubebuilder:default=/etc/haproxy/haproxy.cfg
	ConfigFile string `json:"configFile,omitempty"`

	// BackendName is the backend name in HAProxy config.
	// +optional
	// +kubebuilder:default=k8s-apiserver
	BackendName string `json:"backendName,omitempty"`

	// ReloadCommand is the command to reload HAProxy.
	// +optional
	// +kubebuilder:default="systemctl reload haproxy"
	ReloadCommand string `json:"reloadCommand,omitempty"`
}

// KeepalivedConfig defines Keepalived specific configuration.
type KeepalivedConfig struct {
	// VirtualIP is the virtual IP address.
	// +optional
	VirtualIP string `json:"virtualIP,omitempty"`

	// Interface is the network interface.
	// +optional
	// +kubebuilder:default=eth0
	Interface string `json:"interface,omitempty"`

	// VirtualRouterID is the VRRP router ID.
	// +optional
	// +kubebuilder:default=51
	VirtualRouterID int `json:"virtualRouterID,omitempty"`

	// Priority is the VRRP priority.
	// +optional
	// +kubebuilder:default=100
	Priority int `json:"priority,omitempty"`

	// AdvertInterval is the advertisement interval in seconds.
	// +optional
	// +kubebuilder:default=1
	AdvertInterval int `json:"advertInterval,omitempty"`
}

// F5Config defines F5 BIG-IP specific configuration.
type F5Config struct {
	// Host is the F5 BIG-IP management host.
	// +optional
	Host string `json:"host,omitempty"`

	// Port is the F5 BIG-IP management port.
	// +optional
	// +kubebuilder:default=443
	Port int `json:"port,omitempty"`

	// CredentialsRef references the F5 credentials secret.
	// +optional
	CredentialsRef *corev1.LocalObjectReference `json:"credentialsRef,omitempty"`

	// Partition is the F5 partition.
	// +optional
	// +kubebuilder:default=Common
	Partition string `json:"partition,omitempty"`

	// PoolName is the F5 pool name.
	// +optional
	PoolName string `json:"poolName,omitempty"`

	// VirtualServerName is the F5 virtual server name.
	// +optional
	VirtualServerName string `json:"virtualServerName,omitempty"`
}

// MetalLBConfig defines MetalLB specific configuration.
type MetalLBConfig struct {
	// IPAddressPool is the MetalLB IP address pool name.
	// +optional
	IPAddressPool string `json:"ipAddressPool,omitempty"`

	// LoadBalancerIP is the specific IP to assign.
	// +optional
	LoadBalancerIP string `json:"loadBalancerIP,omitempty"`
}

// BareMetalClusterInitializationStatus provides observations of the BareMetalCluster initialization process.
type BareMetalClusterInitializationStatus struct {
	// Provisioned is true when the infrastructure provider reports that the Cluster's infrastructure is fully provisioned.
	// +optional
	Provisioned *bool `json:"provisioned,omitempty"`
}

// BareMetalClusterStatus defines the observed state of BareMetalCluster.
type BareMetalClusterStatus struct {
	// Ready indicates that the cluster infrastructure is ready.
	// +optional
	Ready bool `json:"ready,omitempty"`

	// Initialization provides observations of the BareMetalCluster initialization process.
	// +optional
	Initialization *BareMetalClusterInitializationStatus `json:"initialization,omitempty"`

	// Conditions defines current service state of the BareMetalCluster.
	// +optional
	//nolint:staticcheck // Conditions deprecated in CAPI v1beta2, will migrate when ready
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

// GetConditions returns the set of conditions for this object.
//nolint:staticcheck // Conditions deprecated in CAPI v1beta2, will migrate when ready
func (c *BareMetalCluster) GetConditions() clusterv1.Conditions {
	return c.Status.Conditions
}

// SetConditions sets the conditions on this object.
//nolint:staticcheck // Conditions deprecated in CAPI v1beta2, will migrate when ready
func (c *BareMetalCluster) SetConditions(conditions clusterv1.Conditions) {
	c.Status.Conditions = conditions
}

// IngressLoadBalancerConfig defines the ingress load balancer configuration.
type IngressLoadBalancerConfig struct {
	// Enabled enables ingress load balancer management.
	// +optional
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// Provider is the load balancer type (haproxy, f5, metal-lb).
	// +optional
	Provider string `json:"provider,omitempty"`

	// HAProxy holds HAProxy specific configuration.
	// +optional
	HAProxy *IngressHAProxyConfig `json:"haproxy,omitempty"`

	// F5 holds F5 BIG-IP specific configuration.
	// +optional
	F5 *IngressF5Config `json:"f5,omitempty"`

	// MetalLB holds MetalLB specific configuration.
	// +optional
	MetalLB *IngressMetalLBConfig `json:"metal-lb,omitempty"`
}

// IngressHAProxyConfig defines HAProxy specific configuration for ingress.
type IngressHAProxyConfig struct {
	// AdminHost is the HAProxy Runtime API host.
	// +optional
	AdminHost string `json:"adminHost,omitempty"`

	// AdminPort is the HAProxy Runtime API port.
	// +optional
	// +kubebuilder:default=9999
	AdminPort int `json:"adminPort,omitempty"`

	// SSHHost is the HAProxy server SSH host.
	// +optional
	SSHHost string `json:"sshHost,omitempty"`

	// SSHPort is the HAProxy server SSH port.
	// +optional
	// +kubebuilder:default=22
	SSHPort int `json:"sshPort,omitempty"`

	// BackendName is the backend name in HAProxy config.
	// +optional
	// +kubebuilder:default=k8s-ingress
	BackendName string `json:"backendName,omitempty"`

	// HTTPPort is the NodePort for HTTP traffic on worker nodes.
	// +optional
	// +kubebuilder:default=30080
	HTTPPort int `json:"httpPort,omitempty"`

	// HTTPSPort is the NodePort for HTTPS traffic on worker nodes.
	// +optional
	// +kubebuilder:default=30443
	HTTPSPort int `json:"httpsPort,omitempty"`
}

// IngressF5Config defines F5 BIG-IP specific configuration for ingress.
type IngressF5Config struct {
	// Host is the F5 BIG-IP management host.
	// +optional
	Host string `json:"host,omitempty"`

	// Port is the F5 BIG-IP management port.
	// +optional
	// +kubebuilder:default=443
	Port int `json:"port,omitempty"`

	// CredentialsRef references the F5 credentials secret.
	// +optional
	CredentialsRef *corev1.LocalObjectReference `json:"credentialsRef,omitempty"`

	// Partition is the F5 partition.
	// +optional
	// +kubebuilder:default=Common
	Partition string `json:"partition,omitempty"`

	// HTTPPoolName is the F5 pool name for HTTP traffic.
	// +optional
	HTTPPoolName string `json:"httpPoolName,omitempty"`

	// HTTPSPoolName is the F5 pool name for HTTPS traffic.
	// +optional
	HTTPSPoolName string `json:"httpsPoolName,omitempty"`

	// HTTPPort is the NodePort for HTTP traffic.
	// +optional
	// +kubebuilder:default=30080
	HTTPPort int `json:"httpPort,omitempty"`

	// HTTPSPort is the NodePort for HTTPS traffic.
	// +optional
	// +kubebuilder:default=30443
	HTTPSPort int `json:"httpsPort,omitempty"`
}

// IngressMetalLBConfig defines MetalLB specific configuration for ingress.
type IngressMetalLBConfig struct {
	// IPAddressPool is the MetalLB IP address pool name.
	// +optional
	IPAddressPool string `json:"ipAddressPool,omitempty"`

	// LoadBalancerIP is the VIP for ingress traffic.
	// +optional
	LoadBalancerIP string `json:"loadBalancerIP,omitempty"`
}

// GatewayAPIConfig defines the Gateway API component configuration.
type GatewayAPIConfig struct {
	// Enabled enables Gateway API component installation.
	// +optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// EnvoyGateway holds Envoy Gateway specific configuration.
	// +optional
	EnvoyGateway *EnvoyGatewayConfig `json:"envoyGateway,omitempty"`

	// MetalLB holds MetalLB specific configuration.
	// +optional
	MetalLB *GatewayMetalLBConfig `json:"metalLB,omitempty"`
}

// EnvoyGatewayConfig defines Envoy Gateway specific configuration.
type EnvoyGatewayConfig struct {
	// ReplicaCount is the number of Envoy Gateway replicas.
	// +optional
	// +kubebuilder:default=2
	ReplicaCount int `json:"replicaCount,omitempty"`

	// NodeSelector for scheduling Envoy Gateway pods.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
}

// GatewayMetalLBConfig defines MetalLB specific configuration for Gateway API.
type GatewayMetalLBConfig struct {
	// Enabled enables MetalLB installation.
	// +optional
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// Mode is the MetalLB mode (layer2 or bgp).
	// +optional
	// +kubebuilder:default=layer2
	Mode string `json:"mode,omitempty"`

	// IPAddressPools defines the IP address pools.
	// +optional
	IPAddressPools []MetalLBIPAddressPool `json:"ipAddressPools,omitempty"`
}

// MetalLBIPAddressPool defines an IP address pool for MetalLB.
type MetalLBIPAddressPool struct {
	// Name is the pool name.
	Name string `json:"name"`

	// Addresses is the list of IP address ranges.
	Addresses []string `json:"addresses"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready",description="BareMetalCluster is Ready"
// +kubebuilder:printcolumn:name="Endpoint",type="string",JSONPath=".spec.controlPlaneEndpoint.host",description="API endpoint host"
// +kubebuilder:printcolumn:name="Port",type="integer",JSONPath=".spec.controlPlaneEndpoint.port",description="API endpoint port"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of BareMetalCluster"

// BareMetalCluster is the Schema for the baremetalclusters API.
type BareMetalCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BareMetalClusterSpec   `json:"spec,omitempty"`
	Status BareMetalClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BareMetalClusterList contains a list of BareMetalCluster.
type BareMetalClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BareMetalCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BareMetalCluster{}, &BareMetalClusterList{})
}
