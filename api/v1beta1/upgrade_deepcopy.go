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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// ==================== ClusterVersion DeepCopy ====================

func (in *ClusterVersion) DeepCopyInto(out *ClusterVersion) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *ClusterVersion) DeepCopy() *ClusterVersion {
	if in == nil {
		return nil
	}
	out := new(ClusterVersion)
	in.DeepCopyInto(out)
	return out
}

func (in *ClusterVersion) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *ClusterVersionSpec) DeepCopyInto(out *ClusterVersionSpec) {
	*out = *in
	out.ClusterRef = in.ClusterRef
	if in.DesiredUpdate != nil {
		in, out := &in.DesiredUpdate, &out.DesiredUpdate
		*out = new(Update)
		**out = **in
	}
}

func (in *ClusterVersionSpec) DeepCopy() *ClusterVersionSpec {
	if in == nil {
		return nil
	}
	out := new(ClusterVersionSpec)
	in.DeepCopyInto(out)
	return out
}

func (in *Update) DeepCopyInto(out *Update) {
	*out = *in
}

func (in *Update) DeepCopy() *Update {
	if in == nil {
		return nil
	}
	out := new(Update)
	in.DeepCopyInto(out)
	return out
}

func (in *ClusterVersionStatus) DeepCopyInto(out *ClusterVersionStatus) {
	*out = *in
	out.Desired = in.Desired
	if in.History != nil {
		in, out := &in.History, &out.History
		*out = make([]UpdateHistory, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]metav1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.AvailableUpdates != nil {
		in, out := &in.AvailableUpdates, &out.AvailableUpdates
		*out = make([]Release, len(*in))
		copy(*out, *in)
	}
	if in.ComponentStatus != nil {
		in, out := &in.ComponentStatus, &out.ComponentStatus
		*out = make([]ComponentStatus, len(*in))
		copy(*out, *in)
	}
}

func (in *ClusterVersionStatus) DeepCopy() *ClusterVersionStatus {
	if in == nil {
		return nil
	}
	out := new(ClusterVersionStatus)
	in.DeepCopyInto(out)
	return out
}

func (in *Release) DeepCopyInto(out *Release) {
	*out = *in
}

func (in *Release) DeepCopy() *Release {
	if in == nil {
		return nil
	}
	out := new(Release)
	in.DeepCopyInto(out)
	return out
}

func (in *UpdateHistory) DeepCopyInto(out *UpdateHistory) {
	*out = *in
	in.StartedTime.DeepCopyInto(&out.StartedTime)
	if in.CompletionTime != nil {
		in, out := &in.CompletionTime, &out.CompletionTime
		*out = (*in).DeepCopy()
	}
}

func (in *UpdateHistory) DeepCopy() *UpdateHistory {
	if in == nil {
		return nil
	}
	out := new(UpdateHistory)
	in.DeepCopyInto(out)
	return out
}

func (in *ComponentStatus) DeepCopyInto(out *ComponentStatus) {
	*out = *in
}

func (in *ComponentStatus) DeepCopy() *ComponentStatus {
	if in == nil {
		return nil
	}
	out := new(ComponentStatus)
	in.DeepCopyInto(out)
	return out
}

func (in *ClusterVersionList) DeepCopyInto(out *ClusterVersionList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ClusterVersion, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *ClusterVersionList) DeepCopy() *ClusterVersionList {
	if in == nil {
		return nil
	}
	out := new(ClusterVersionList)
	in.DeepCopyInto(out)
	return out
}

func (in *ClusterVersionList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// ==================== ReleaseImage DeepCopy ====================

func (in *ReleaseImage) DeepCopyInto(out *ReleaseImage) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

func (in *ReleaseImage) DeepCopy() *ReleaseImage {
	if in == nil {
		return nil
	}
	out := new(ReleaseImage)
	in.DeepCopyInto(out)
	return out
}

func (in *ReleaseImage) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *ReleaseImageSpec) DeepCopyInto(out *ReleaseImageSpec) {
	*out = *in
	if in.Channels != nil {
		in, out := &in.Channels, &out.Channels
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.PreviousVersions != nil {
		in, out := &in.PreviousVersions, &out.PreviousVersions
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	in.Components.DeepCopyInto(&out.Components)
	if in.UpgradeGraph != nil {
		in, out := &in.UpgradeGraph, &out.UpgradeGraph
		*out = make([]UpgradePhase, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *ReleaseImageSpec) DeepCopy() *ReleaseImageSpec {
	if in == nil {
		return nil
	}
	out := new(ReleaseImageSpec)
	in.DeepCopyInto(out)
	return out
}

func (in *ReleaseComponentVersions) DeepCopyInto(out *ReleaseComponentVersions) {
	*out = *in
	in.Kubernetes.DeepCopyInto(&out.Kubernetes)
	out.Containerd = in.Containerd
	out.Helm = in.Helm
	out.CNIPlugins = in.CNIPlugins
}

func (in *KubernetesComponent) DeepCopyInto(out *KubernetesComponent) {
	*out = *in
	if in.Platforms != nil {
		in, out := &in.Platforms, &out.Platforms
		*out = make(map[string]K8SPlatform, len(*in))
		for key, val := range *in {
			(*out)[key] = *val.DeepCopy()
		}
	}
}

func (in *K8SPlatform) DeepCopyInto(out *K8SPlatform) {
	*out = *in
	if in.Architectures != nil {
		in, out := &in.Architectures, &out.Architectures
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.Packages != nil {
		in, out := &in.Packages, &out.Packages
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

func (in *K8SPlatform) DeepCopy() *K8SPlatform {
	if in == nil {
		return nil
	}
	out := new(K8SPlatform)
	in.DeepCopyInto(out)
	return out
}

func (in *BinaryComponent) DeepCopyInto(out *BinaryComponent) {
	*out = *in
	if in.Architectures != nil {
		in, out := &in.Architectures, &out.Architectures
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	out.Files = in.Files
}

func (in *UpgradePhase) DeepCopyInto(out *UpgradePhase) {
	*out = *in
	if in.RollingUpdate != nil {
		in, out := &in.RollingUpdate, &out.RollingUpdate
		*out = new(RollingUpdate)
		**out = **in
	}
	if in.Components != nil {
		in, out := &in.Components, &out.Components
		*out = make([]UpgradeComponent, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *UpgradePhase) DeepCopy() *UpgradePhase {
	if in == nil {
		return nil
	}
	out := new(UpgradePhase)
	in.DeepCopyInto(out)
	return out
}

func (in *UpgradeComponent) DeepCopyInto(out *UpgradeComponent) {
	*out = *in
	if in.Manifests != nil {
		in, out := &in.Manifests, &out.Manifests
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.Scripts != nil {
		in, out := &in.Scripts, &out.Scripts
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.DependsOn != nil {
		in, out := &in.DependsOn, &out.DependsOn
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.HealthCheck != nil {
		in, out := &in.HealthCheck, &out.HealthCheck
		*out = new(HealthCheck)
		**out = **in
	}
}

func (in *UpgradeComponent) DeepCopy() *UpgradeComponent {
	if in == nil {
		return nil
	}
	out := new(UpgradeComponent)
	in.DeepCopyInto(out)
	return out
}

func (in *HealthCheck) DeepCopyInto(out *HealthCheck) {
	*out = *in
	out.Timeout = in.Timeout
}

func (in *HealthCheck) DeepCopy() *HealthCheck {
	if in == nil {
		return nil
	}
	out := new(HealthCheck)
	in.DeepCopyInto(out)
	return out
}

func (in *RollingUpdate) DeepCopyInto(out *RollingUpdate) {
	*out = *in
}

func (in *RollingUpdate) DeepCopy() *RollingUpdate {
	if in == nil {
		return nil
	}
	out := new(RollingUpdate)
	in.DeepCopyInto(out)
	return out
}

func (in *ReleaseImageStatus) DeepCopyInto(out *ReleaseImageStatus) {
	*out = *in
}

func (in *ReleaseImageStatus) DeepCopy() *ReleaseImageStatus {
	if in == nil {
		return nil
	}
	out := new(ReleaseImageStatus)
	in.DeepCopyInto(out)
	return out
}

func (in *ReleaseImageList) DeepCopyInto(out *ReleaseImageList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ReleaseImage, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *ReleaseImageList) DeepCopy() *ReleaseImageList {
	if in == nil {
		return nil
	}
	out := new(ReleaseImageList)
	in.DeepCopyInto(out)
	return out
}

func (in *ReleaseImageList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// ==================== UpgradePath DeepCopy ====================

func (in *UpgradePath) DeepCopyInto(out *UpgradePath) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

func (in *UpgradePath) DeepCopy() *UpgradePath {
	if in == nil {
		return nil
	}
	out := new(UpgradePath)
	in.DeepCopyInto(out)
	return out
}

func (in *UpgradePath) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *UpgradePathSpec) DeepCopyInto(out *UpgradePathSpec) {
	*out = *in
	in.Graph.DeepCopyInto(&out.Graph)
	in.Rules.DeepCopyInto(&out.Rules)
}

func (in *UpgradePathSpec) DeepCopy() *UpgradePathSpec {
	if in == nil {
		return nil
	}
	out := new(UpgradePathSpec)
	in.DeepCopyInto(out)
	return out
}

func (in *UpgradeGraphData) DeepCopyInto(out *UpgradeGraphData) {
	*out = *in
	if in.Edges != nil {
		in, out := &in.Edges, &out.Edges
		*out = make([]GraphEdge, len(*in))
		copy(*out, *in)
	}
}

func (in *UpgradeGraphData) DeepCopy() *UpgradeGraphData {
	if in == nil {
		return nil
	}
	out := new(UpgradeGraphData)
	in.DeepCopyInto(out)
	return out
}

func (in *GraphEdge) DeepCopyInto(out *GraphEdge) {
	*out = *in
}

func (in *GraphEdge) DeepCopy() *GraphEdge {
	if in == nil {
		return nil
	}
	out := new(GraphEdge)
	in.DeepCopyInto(out)
	return out
}

func (in *CompatibilityRules) DeepCopyInto(out *CompatibilityRules) {
	*out = *in
	if in.BlockedUpgrades != nil {
		in, out := &in.BlockedUpgrades, &out.BlockedUpgrades
		*out = make([]BlockedUpgrade, len(*in))
		copy(*out, *in)
	}
}

func (in *CompatibilityRules) DeepCopy() *CompatibilityRules {
	if in == nil {
		return nil
	}
	out := new(CompatibilityRules)
	in.DeepCopyInto(out)
	return out
}

func (in *BlockedUpgrade) DeepCopyInto(out *BlockedUpgrade) {
	*out = *in
}

func (in *BlockedUpgrade) DeepCopy() *BlockedUpgrade {
	if in == nil {
		return nil
	}
	out := new(BlockedUpgrade)
	in.DeepCopyInto(out)
	return out
}

func (in *UpgradePathStatus) DeepCopyInto(out *UpgradePathStatus) {
	*out = *in
	in.LastSyncTime.DeepCopyInto(&out.LastSyncTime)
}

func (in *UpgradePathStatus) DeepCopy() *UpgradePathStatus {
	if in == nil {
		return nil
	}
	out := new(UpgradePathStatus)
	in.DeepCopyInto(out)
	return out
}

func (in *UpgradePathList) DeepCopyInto(out *UpgradePathList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]UpgradePath, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *UpgradePathList) DeepCopy() *UpgradePathList {
	if in == nil {
		return nil
	}
	out := new(UpgradePathList)
	in.DeepCopyInto(out)
	return out
}

func (in *UpgradePathList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// ==================== ReleaseCatalog DeepCopy ====================

func (in *ReleaseCatalog) DeepCopyInto(out *ReleaseCatalog) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *ReleaseCatalog) DeepCopy() *ReleaseCatalog {
	if in == nil {
		return nil
	}
	out := new(ReleaseCatalog)
	in.DeepCopyInto(out)
	return out
}

func (in *ReleaseCatalog) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *ReleaseCatalogSpec) DeepCopyInto(out *ReleaseCatalogSpec) {
	*out = *in
	out.SyncInterval = in.SyncInterval
}

func (in *ReleaseCatalogSpec) DeepCopy() *ReleaseCatalogSpec {
	if in == nil {
		return nil
	}
	out := new(ReleaseCatalogSpec)
	in.DeepCopyInto(out)
	return out
}

func (in *ReleaseCatalogStatus) DeepCopyInto(out *ReleaseCatalogStatus) {
	*out = *in
	in.LastSyncTime.DeepCopyInto(&out.LastSyncTime)
	if in.Releases != nil {
		in, out := &in.Releases, &out.Releases
		*out = make([]ReleaseEntry, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.Channels != nil {
		in, out := &in.Channels, &out.Channels
		*out = make(map[string][]ChannelVersion, len(*in))
		for key, val := range *in {
			var outVal []ChannelVersion
			if val == nil {
				(*out)[key] = nil
			} else {
				in, out := &val, &outVal
				*out = make([]ChannelVersion, len(*in))
				copy(*out, *in)
			}
			(*out)[key] = outVal
		}
	}
}

func (in *ReleaseCatalogStatus) DeepCopy() *ReleaseCatalogStatus {
	if in == nil {
		return nil
	}
	out := new(ReleaseCatalogStatus)
	in.DeepCopyInto(out)
	return out
}

func (in *ReleaseEntry) DeepCopyInto(out *ReleaseEntry) {
	*out = *in
	if in.Channels != nil {
		in, out := &in.Channels, &out.Channels
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

func (in *ReleaseEntry) DeepCopy() *ReleaseEntry {
	if in == nil {
		return nil
	}
	out := new(ReleaseEntry)
	in.DeepCopyInto(out)
	return out
}

func (in *ChannelVersion) DeepCopyInto(out *ChannelVersion) {
	*out = *in
}

func (in *ChannelVersion) DeepCopy() *ChannelVersion {
	if in == nil {
		return nil
	}
	out := new(ChannelVersion)
	in.DeepCopyInto(out)
	return out
}

func (in *ReleaseCatalogList) DeepCopyInto(out *ReleaseCatalogList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ReleaseCatalog, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *ReleaseCatalogList) DeepCopy() *ReleaseCatalogList {
	if in == nil {
		return nil
	}
	out := new(ReleaseCatalogList)
	in.DeepCopyInto(out)
	return out
}

func (in *ReleaseCatalogList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}
