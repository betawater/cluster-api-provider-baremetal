# Cluster API Provider Bare Metal (CAPBM)

Cluster API Provider Bare Metal (CAPBM) is an infrastructure provider for [Cluster API](https://cluster-api.sigs.k8s.io/) that enables management of bare metal machines as Kubernetes cluster nodes.

## Features

- **SSH-based Machine Management**: Connect to pre-provisioned bare metal machines via SSH
- **Pre-flight Checks**: Validate OS, kernel, disk, memory, and network before joining cluster
- **ClusterClass Support**: Define cluster templates with variables and patches for flexible deployment
- **Power Management**: Optional IPMI/Redfish integration for machine power control
- **Connection Pooling**: Efficient SSH connection management with idle timeout

## Quick Start

### Prerequisites

- Kubernetes v1.28+ management cluster
- clusterctl v1.8+
- Pre-provisioned bare metal machines with:
  - SSH access enabled
  - Supported OS (Ubuntu, CentOS, Rocky, AlmaLinux, Debian)
  - Minimum 2GB RAM, 20GB disk

### Installation

```bash
# Install CAPI core components and CAPBM provider
clusterctl init --core cluster-api --bootstrap kubeadm --control-plane kubeadm --infrastructure baremetal

# Deploy ClusterClass templates
kubectl apply -f config/clusterclass/
```

### Create a Cluster

```bash
# Create SSH credentials secret
kubectl create secret generic baremetal-ssh-credentials \
  --from-literal=username=root \
  --from-literal=password=yourpassword

# Generate cluster manifest
clusterctl generate cluster my-cluster \
  --from templates/clusterclass/baremetal-clusterclass-v0.1.0.yaml \
  --variable CONTROL_PLANE_ENDPOINT_HOST=lb.example.com \
  --variable SSH_CREDENTIALS_SECRET=baremetal-ssh-credentials \
  > cluster.yaml

# Apply cluster manifest
kubectl apply -f cluster.yaml
```

### Monitor Cluster Creation

```bash
# Watch cluster status
clusterctl describe cluster my-cluster

# Get workload cluster kubeconfig
clusterctl get kubeconfig my-cluster > workload-kubeconfig
kubectl --kubeconfig workload-kubeconfig get nodes
```

## ClusterClass Variables

| Variable | Type | Required | Default | Description |
|----------|------|----------|---------|-------------|
| controlPlaneEndpoint | object | Yes | - | Load balancer endpoint (host, port) |
| credentialsSecret | string | Yes | - | SSH credentials Secret name |
| kubernetesVersion | string | Yes | - | Kubernetes version (e.g., v1.31.0) |
| podCIDR | string | No | 10.244.0.0/16 | Pod network CIDR |
| serviceCIDR | string | No | 10.96.0.0/12 | Service network CIDR |
| preFlightChecks | object | No | enabled: true | Pre-flight check configuration |

## Development

```bash
# Build the project
make build

# Run tests
make test

# Run locally
make run

# Build and push Docker image
make docker-build docker-push IMG=your-registry/capbm:v0.1.0

# Deploy to cluster
make deploy IMG=your-registry/capbm:v0.1.0
```

## Project Structure

```
├── api/v1beta2/          # CRD type definitions
├── cmd/                  # Entry point
├── config/
│   ├── crd/              # CRD YAML definitions
│   ├── clusterclass/     # ClusterClass templates
│   ├── rbac/             # RBAC configuration
│   └── manager/          # Controller deployment
├── internal/
│   ├── controllers/      # Reconciler implementations
│   └── ssh/              # SSH connection management
├── templates/clusterclass/ # clusterctl templates
└── test/                 # E2E and unit tests
```

## License

Copyright 2024 The CAPBM Authors.

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) for details.
