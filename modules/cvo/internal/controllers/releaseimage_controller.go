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

package controllers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	cfov1 "github.com/BetaWater/cluster-api-provider-baremetal/modules/cvo/api/v1beta1"
	"github.com/BetaWater/cluster-api-provider-baremetal/modules/cvo/internal/upgrader"
)

const (
	ReleaseImageFinalizer = "releaseimage.cvo.capbm.io"
)

// ReleaseImageReconciler reconciles a ReleaseImage object
type ReleaseImageReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=cvo.capbm.io,resources=releaseimages,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cvo.capbm.io,resources=releaseimages/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cvo.capbm.io,resources=releaseimages/finalizers,verbs=update
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch

func (r *ReleaseImageReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// Fetch the ReleaseImage
	ri := &cfov1.ReleaseImage{}
	if err := r.Get(ctx, req.NamespacedName, ri); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion
	if !ri.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, ri)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(ri, ReleaseImageFinalizer) {
		controllerutil.AddFinalizer(ri, ReleaseImageFinalizer)
		if err := r.Update(ctx, ri); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Verify content hash if specified
	if ri.Spec.ContentHash != "" && !ri.Status.Verified {
		if err := r.verifyContentHash(ctx, ri); err != nil {
			log.Error(err, "Failed to verify content hash")
			return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
		}
		ri.Status.Verified = true
		if err := r.Status().Update(ctx, ri); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Count manifests if not already done
	if ri.Status.ManifestCount == 0 {
		ri.Status.ManifestCount = r.countManifests(ctx, ri)
		if err := r.Status().Update(ctx, ri); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *ReleaseImageReconciler) reconcileDelete(ctx context.Context, ri *cfov1.ReleaseImage) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Deleting ReleaseImage")

	// Remove finalizer
	controllerutil.RemoveFinalizer(ri, ReleaseImageFinalizer)
	return ctrl.Result{}, r.Update(ctx, ri)
}

func (r *ReleaseImageReconciler) verifyContentHash(ctx context.Context, ri *cfov1.ReleaseImage) error {
	if ri.Spec.Image == "" {
		return fmt.Errorf("release image reference is empty")
	}

	// Pull OCI image and calculate content hash
	puller := upgrader.NewOCIPuller("")
	dir, err := puller.GetImageDir(ctx, ri.Spec.Image)
	if err != nil {
		return fmt.Errorf("failed to pull release image: %w", err)
	}

	// Calculate SHA256 hash of all content
	hash := sha256.New()
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() {
			if closeErr := f.Close(); closeErr != nil {
				// Log close error but don't fail the hash calculation
			}
		}()
		if _, err := io.Copy(hash, f); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to calculate content hash: %w", err)
	}

	calculatedHash := "sha256:" + hex.EncodeToString(hash.Sum(nil))
	if calculatedHash != ri.Spec.ContentHash {
		return fmt.Errorf("content hash mismatch: expected %s, got %s", ri.Spec.ContentHash, calculatedHash)
	}

	return nil
}

func (r *ReleaseImageReconciler) countManifests(ctx context.Context, ri *cfov1.ReleaseImage) int {
	if ri.Spec.Image == "" {
		// Fallback: count addons with type "manifest"
		count := 0
		for _, addon := range ri.Spec.Addons {
			if addon.Type == cfov1.AddonTypeManifest {
				count++
			}
		}
		return count
	}

	// Pull OCI image and count manifest files
	puller := upgrader.NewOCIPuller("")
	dir, err := puller.GetImageDir(ctx, ri.Spec.Image)
	if err != nil {
		// Fallback to addon count on pull failure
		count := 0
		for _, addon := range ri.Spec.Addons {
			if addon.Type == cfov1.AddonTypeManifest {
				count++
			}
		}
		return count
	}

	// Count YAML/YML files in manifests directory
	manifestsDir := filepath.Join(dir, "manifests")
	count := 0
	if _, err := os.Stat(manifestsDir); err == nil {
		err = filepath.Walk(manifestsDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext == ".yaml" || ext == ".yml" {
				count++
			}
			return nil
		})
		if err != nil {
			// Fallback to addon count
			count = 0
			for _, addon := range ri.Spec.Addons {
				if addon.Type == cfov1.AddonTypeManifest {
					count++
				}
			}
		}
	}

	return count
}

// SetupWithManager sets up the controller with the Manager.
func (r *ReleaseImageReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cfov1.ReleaseImage{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
