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

package images

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/BetaWater/cluster-api-provider-baremetal/modules/cvo/pkg/ssh"
)

// LoadResult holds the result of image loading.
type LoadResult struct {
	Completed bool     `json:"completed"`
	Success   bool     `json:"success"`
	Loaded    []string `json:"loaded,omitempty"`
	Failed    []string `json:"failed,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// ImageLoader loads container images from a release server.
type ImageLoader struct {
	releaseServer string
	namespace     string
	sshConn       *ssh.SSHConnection
}

// NewImageLoader creates a new image loader.
func NewImageLoader(releaseServer, namespace string) *ImageLoader {
	if namespace == "" {
		namespace = "k8s.io"
	}
	return &ImageLoader{
		releaseServer: strings.TrimRight(releaseServer, "/"),
		namespace:     namespace,
	}
}

// WithSSHConnection sets the SSH connection for executing load scripts.
func (l *ImageLoader) WithSSHConnection(conn *ssh.SSHConnection) *ImageLoader {
	l.sshConn = conn
	return l
}

// LoadComponentImages loads all images for a component.
func (l *ImageLoader) LoadComponentImages(ctx context.Context, component, version string, imageList []string) (*LoadResult, error) {
	if len(imageList) == 0 {
		return &LoadResult{Completed: true, Success: true}, nil
	}

	imagePath := fmt.Sprintf("images/%s/%s", component, version)
	return l.loadImages(ctx, imagePath, imageList)
}

// LoadKubernetesImages loads all Kubernetes component images.
func (l *ImageLoader) LoadKubernetesImages(ctx context.Context, version string) (*LoadResult, error) {
	k8sImages := []string{
		"kube-apiserver.tar",
		"kube-controller-manager.tar",
		"kube-scheduler.tar",
		"kube-proxy.tar",
		"coredns.tar",
		"etcd.tar",
		"pause.tar",
	}

	imagePath := fmt.Sprintf("images/kubernetes/%s", version)
	return l.loadImages(ctx, imagePath, k8sImages)
}

// LoadCNIImages loads CNI component images.
func (l *ImageLoader) LoadCNIImages(ctx context.Context, cniType, version string) (*LoadResult, error) {
	var imageList []string

	switch cniType {
	case "calico":
		imageList = []string{
			"calico-node.tar",
			"calico-kube-controllers.tar",
			"calico-cni.tar",
		}
	case "cilium":
		imageList = []string{
			"cilium.tar",
			"cilium-operator.tar",
			"hubble-relay.tar",
		}
	case "flannel":
		imageList = []string{
			"flannel.tar",
		}
	default:
		return &LoadResult{Completed: true, Success: true}, nil
	}

	imagePath := fmt.Sprintf("images/%s/%s", cniType, version)
	return l.loadImages(ctx, imagePath, imageList)
}

// LoadCSIImages loads CSI component images.
func (l *ImageLoader) LoadCSIImages(ctx context.Context, csiType, version string) (*LoadResult, error) {
	var imageList []string

	switch csiType {
	case "ceph-csi":
		imageList = []string{
			"cephcsi.tar",
			"csi-attacher.tar",
			"csi-provisioner.tar",
			"csi-snapshotter.tar",
			"csi-resizer.tar",
			"csi-node-driver-registrar.tar",
		}
	case "local-path-provisioner":
		imageList = []string{
			"local-path-provisioner.tar",
			"helper.tar",
		}
	case "nfs-csi":
		imageList = []string{
			"nfs.tar",
			"csi-node-driver-registrar.tar",
		}
	default:
		return &LoadResult{Completed: true, Success: true}, nil
	}

	imagePath := fmt.Sprintf("images/%s/%s", csiType, version)
	return l.loadImages(ctx, imagePath, imageList)
}

// LoadGatewayImages loads Gateway API component images.
func (l *ImageLoader) LoadGatewayImages(ctx context.Context, version string) (*LoadResult, error) {
	imageList := []string{
		"envoy-gateway.tar",
		"envoy-proxy.tar",
	}

	imagePath := fmt.Sprintf("images/envoy-gateway/%s", version)
	return l.loadImages(ctx, imagePath, imageList)
}

// LoadMetalLBImages loads MetalLB component images.
func (l *ImageLoader) LoadMetalLBImages(ctx context.Context, version string) (*LoadResult, error) {
	imageList := []string{
		"metallb-controller.tar",
		"metallb-speaker.tar",
	}

	imagePath := fmt.Sprintf("images/metallb/%s", version)
	return l.loadImages(ctx, imagePath, imageList)
}

// loadImages loads images from the release server.
func (l *ImageLoader) loadImages(ctx context.Context, imagePath string, imageList []string) (*LoadResult, error) {
	result := &LoadResult{
		Loaded: make([]string, 0),
		Failed: make([]string, 0),
	}

	for _, imageTar := range imageList {
		tarURL := fmt.Sprintf("%s/%s/%s", l.releaseServer, imagePath, imageTar)
		dest := filepath.Join("/tmp", imageTar)

		// Build script to download and import
		script := fmt.Sprintf(`#!/bin/bash
set -euo pipefail

IMAGE_URL="%s"
DEST="%s"
NAMESPACE="%s"

echo "Downloading: $(basename $DEST)"
curl -fsSL "$IMAGE_URL" -o "$DEST"

echo "Importing: $(basename $DEST)"
ctr -n "$NAMESPACE" images import "$DEST"
rm -f "$DEST"

echo "Successfully imported: $(basename $DEST)"
`, tarURL, dest, l.namespace)

		// Execute script via SSH if connection is available
		if l.sshConn != nil {
			res, err := l.sshConn.ExecuteScript(ctx, script)
			if err != nil {
				result.Failed = append(result.Failed, imageTar)
				result.Error = fmt.Sprintf("failed to load %s: %v", imageTar, err)
				continue
			}
			if res.ExitCode != 0 {
				result.Failed = append(result.Failed, imageTar)
				result.Error = fmt.Sprintf("failed to load %s: %s", imageTar, res.Stderr)
				continue
			}
		}

		result.Loaded = append(result.Loaded, imageTar)
	}

	result.Completed = true
	result.Success = len(result.Failed) == 0

	return result, nil
}

// GenerateLoadScript generates a bash script to load images.
func (l *ImageLoader) GenerateLoadScript(component, version string, imageList []string) string {
	if len(imageList) == 0 {
		return ""
	}

	imagePath := fmt.Sprintf("images/%s/%s", component, version)

	var scriptParts []string
	scriptParts = append(scriptParts, fmt.Sprintf(`#!/bin/bash
set -euo pipefail

RELEASE_SERVER="%s"
NAMESPACE="%s"
IMAGE_PATH="%s"

echo "=== Loading %s v%s images ==="`, l.releaseServer, l.namespace, imagePath, component, version))

	for _, imageTar := range imageList {
		scriptParts = append(scriptParts, fmt.Sprintf(`
# Load %s
echo "Downloading: %s"
curl -fsSL "${RELEASE_SERVER}/${IMAGE_PATH}/%s" -o "/tmp/%s"
echo "Importing: %s"
ctr -n "$NAMESPACE" images import "/tmp/%s"
rm -f "/tmp/%s"
echo "Successfully imported: %s"
`, imageTar, imageTar, imageTar, imageTar, imageTar, imageTar, imageTar, imageTar))
	}

	scriptParts = append(scriptParts, fmt.Sprintf(`
echo "=== %s v%s images loaded ==="`, component, version))

	return strings.Join(scriptParts, "\n")
}

// GenerateKubernetesLoadScript generates a bash script to load Kubernetes images.
func (l *ImageLoader) GenerateKubernetesLoadScript(version string) string {
	k8sImages := []string{
		"kube-apiserver.tar",
		"kube-controller-manager.tar",
		"kube-scheduler.tar",
		"kube-proxy.tar",
		"coredns.tar",
		"etcd.tar",
		"pause.tar",
	}

	return l.GenerateLoadScript("kubernetes", version, k8sImages)
}

// GenerateCNILoadScript generates a bash script to load CNI images.
func (l *ImageLoader) GenerateCNILoadScript(cniType, version string) string {
	var imageList []string

	switch cniType {
	case "calico":
		imageList = []string{
			"calico-node.tar",
			"calico-kube-controllers.tar",
			"calico-cni.tar",
		}
	case "cilium":
		imageList = []string{
			"cilium.tar",
			"cilium-operator.tar",
			"hubble-relay.tar",
		}
	case "flannel":
		imageList = []string{
			"flannel.tar",
		}
	default:
		return ""
	}

	return l.GenerateLoadScript(cniType, version, imageList)
}

// GenerateCSILoadScript generates a bash script to load CSI images.
func (l *ImageLoader) GenerateCSILoadScript(csiType, version string) string {
	var imageList []string

	switch csiType {
	case "ceph-csi":
		imageList = []string{
			"cephcsi.tar",
			"csi-attacher.tar",
			"csi-provisioner.tar",
			"csi-snapshotter.tar",
			"csi-resizer.tar",
			"csi-node-driver-registrar.tar",
		}
	case "local-path-provisioner":
		imageList = []string{
			"local-path-provisioner.tar",
			"helper.tar",
		}
	case "nfs-csi":
		imageList = []string{
			"nfs.tar",
			"csi-node-driver-registrar.tar",
		}
	default:
		return ""
	}

	return l.GenerateLoadScript(csiType, version, imageList)
}
