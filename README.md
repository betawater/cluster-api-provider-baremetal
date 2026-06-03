# Cluster API Provider Bare Metal (CAPBM)

Cluster API Provider Bare Metal (CAPBM) is an infrastructure provider for [Cluster API](https://cluster-api.sigs.k8s.io/) that enables management of bare metal machines as Kubernetes cluster nodes.

## Architecture

This project contains two independent modules:

| Module | API Group | Purpose |
|--------|-----------|---------|
| **CVO** (Cluster Version Operator) | `cvo.capbm.io` | Cluster version management and upgrade coordination |
| **CAPBM** | `infrastructure.cluster.x-k8s.io` | Bare metal infrastructure provider |

## Features

- **SSH-based Machine Management**: Connect to pre-provisioned bare metal machines via SSH
- **Pre-flight Checks**: Validate OS, kernel, disk, memory, and network before joining cluster
- **ClusterClass Support**: Define cluster templates with variables and patches for flexible deployment
- **Power Management**: Optional IPMI/Redfish integration for machine power control
- **Connection Pooling**: Efficient SSH connection management with idle timeout
- **Cluster Version Management**: Automated cluster upgrade orchestration with CVO
- **Addon Lifecycle Management**: CNI, CSI, and other addon installation and upgrades

## Quick Start

### Prerequisites

- Kubernetes v1.31+ management cluster
- Go 1.26+ (for development)
- kustomize v5.4.3+
- Pre-provisioned bare metal machines with:
  - SSH access enabled
  - Supported OS (Ubuntu, CentOS, Rocky, AlmaLinux, Debian)
  - Minimum 2GB RAM, 20GB disk

### Installation

```bash
# Install CAPI core components
clusterctl init --core cluster-api --bootstrap kubeadm --control-plane kubeadm

# Install CAPBM provider
kubectl apply -k modules/capbm/config/default/

# Install CVO (version operator)
kubectl apply -k modules/cvo/config/default/

# Deploy ClusterClass templates
kubectl apply -k modules/capbm/config/clusterclass/
```

### Create a Cluster

```bash
# Create SSH credentials secret
kubectl create secret generic baremetal-ssh-credentials \
  --from-literal=username=root \
  --from-literal=password=yourpassword

# Generate cluster manifest
clusterctl generate cluster my-cluster \
  --from modules/capbm/templates/clusterclass/baremetal-clusterclass-v0.1.0.yaml \
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

## Development

### Build

```bash
# Build both modules
make build

# Build individually
make build-capbm
make build-cvo
```

### Run Locally

```bash
# Run CAPBM manager locally
make run-capbm

# Run CVO manager locally
make run-cvo
```

### Test

```bash
# Run all tests
make test

# Run tests for specific module
cd modules/capbm && go test ./...
cd modules/cvo && go test ./...
```

### Docker

```bash
# Build images
make docker-build-capbm CAPBM_IMG=your-registry/capbm-manager:v0.1.0
make docker-build-cvo CVO_IMG=your-registry/cvo-manager:v0.1.0

# Push images
make docker-push-capbm
make docker-push-cvo
```

### Deploy

```bash
# Install CRDs
make install-capbm
make install-cvo

# Deploy controllers
make deploy-capbm CAPBM_IMG=your-registry/capbm-manager:v0.1.0
make deploy-cvo CVO_IMG=your-registry/cvo-manager:v0.1.0
```

### Lint

```bash
# Run golangci-lint for both modules
cd modules/capbm && golangci-lint run
cd modules/cvo && golangci-lint run
```

## Project Structure

```
├── modules/
│   ├── cvo/                    # Cluster Version Operator
│   │   ├── go.mod
│   │   ├── api/v1beta1/        # CVO API types
│   │   ├── cmd/manager/        # CVO entry point
│   │   ├── internal/           # CVO controllers & logic
│   │   │   ├── controllers/
│   │   │   ├── upgrader/
│   │   │   ├── addon/
│   │   │   └── registry/
│   │   ├── pkg/ssh/            # Public SSH package
│   │   └── config/             # CVO deployment configs
│   │       ├── crd/bases/      # Generated CRD YAMLs
│   │       ├── rbac/
│   │       └── manager/
│   │
│   └── capbm/                  # CAPBM Infrastructure Provider
│       ├── go.mod
│       ├── api/v1beta1/        # CAPBM API types
│       ├── cmd/manager/        # CAPBM entry point
│       ├── internal/           # Controllers, SSH, LB, etc.
│       │   ├── controllers/
│       │   ├── ssh/
│       │   ├── installer/
│       │   ├── lb/
│       │   ├── cni/
│       │   ├── csi/
│       │   └── ...
│       └── config/             # CAPBM deployment configs
│           ├── crd/bases/      # Generated CRD YAMLs
│           ├── rbac/
│           ├── manager/
│           └── clusterclass/   # ClusterClass templates
│
├── docs/                       # Design documentation
├── hack/                       # Helper scripts
├── templates/                  # Templates
├── test/                       # E2E tests
├── go.work                     # Go workspace definition
├── Makefile
├── Dockerfile.cvo              # CVO Docker build
└── Dockerfile.capbm            # CAPBM Docker build
```

## API Groups

### CVO Module (`cvo.capbm.io`)

| CRD | Description |
|-----|-------------|
| `ClusterVersion` | Cluster version status and upgrade target |
| `ReleaseImage` | Release image and component definitions |
| `UpgradePath` | Upgrade path and compatibility rules |
| `ReleaseCatalog` | Available release versions catalog |
| `ClusterAddon` | Cluster addon lifecycle management |

### CAPBM Module (`infrastructure.cluster.x-k8s.io`)

| CRD | Description |
|-----|-------------|
| `BareMetalCluster` | Bare metal cluster infrastructure |
| `BareMetalMachine` | Bare metal machine instance |
| `BareMetalHostInventory` | Host pool management |
| `BareMetalClusterTemplate` | Cluster template |
| `BareMetalMachineTemplate` | Machine template |

## ClusterClass Variables

| Variable | Type | Required | Default | Description |
|----------|------|----------|---------|-------------|
| controlPlaneEndpoint | object | Yes | - | Load balancer endpoint (host, port) |
| credentialsSecret | string | Yes | - | SSH credentials Secret name |
| kubernetesVersion | string | Yes | - | Kubernetes version (e.g., v1.31.0) |
| podCIDR | string | No | 10.244.0.0/16 | Pod network CIDR |
| serviceCIDR | string | No | 10.96.0.0/12 | Service network CIDR |
| preFlightChecks | object | No | enabled: true | Pre-flight check configuration |

## License

Copyright 2024 The CAPBM Authors.

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) for details.
