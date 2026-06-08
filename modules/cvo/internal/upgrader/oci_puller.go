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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/file"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/retry"

	cfov1 "github.com/BetaWater/cluster-api-provider-baremetal/modules/cvo/api/v1beta1"
)

const (
	DefaultCatalogImage     = ""
	DefaultUpgradePathImage = ""
	MediaTypeReleaseJSON    = "application/vnd.capbm.release.json.v1+json"
	MediaTypeBinaryLayer    = "application/vnd.capbm.binary.layer.v1.tar+gzip"
	MediaTypeChartLayer     = "application/vnd.capbm.chart.layer.v1.tar+gzip"
	MediaTypeManifestLayer  = "application/vnd.capbm.manifest.layer.v1.yaml"
	MediaTypeScriptLayer    = "application/vnd.capbm.script.layer.v1.sh"
)

type OCIPuller struct {
	workDir string
	auth    *AuthConfig
}

type AuthConfig struct {
	Username string
	Password string
}

func NewOCIPuller(workDir string) *OCIPuller {
	if workDir == "" {
		workDir = "/tmp/capbm-upgrade"
	}
	return &OCIPuller{workDir: workDir}
}

func (p *OCIPuller) WithAuth(username, password string) *OCIPuller {
	p.auth = &AuthConfig{
		Username: username,
		Password: password,
	}
	return p
}

func (p *OCIPuller) PullAndParseCatalog(ctx context.Context, image string) (*cfov1.ReleaseCatalogStatus, error) {
	dir, err := p.pullImage(ctx, image, "catalog")
	if err != nil {
		return nil, fmt.Errorf("failed to pull catalog image: %w", err)
	}

	catalogData, err := os.ReadFile(filepath.Join(dir, "catalog.json"))
	if err != nil {
		return nil, fmt.Errorf("failed to read catalog.json: %w", err)
	}

	var status cfov1.ReleaseCatalogStatus
	if err := json.Unmarshal(catalogData, &status); err != nil {
		return nil, fmt.Errorf("failed to parse catalog.json: %w", err)
	}

	return &status, nil
}

func (p *OCIPuller) PullAndParseUpgradePath(ctx context.Context, image string) (*cfov1.UpgradePathSpec, error) {
	dir, err := p.pullImage(ctx, image, "upgradepath")
	if err != nil {
		return nil, fmt.Errorf("failed to pull upgrade path image: %w", err)
	}

	pathData, err := os.ReadFile(filepath.Join(dir, "upgrade-path.json"))
	if err != nil {
		return nil, fmt.Errorf("failed to read upgrade-path.json: %w", err)
	}

	var spec cfov1.UpgradePathSpec
	if err := json.Unmarshal(pathData, &spec); err != nil {
		return nil, fmt.Errorf("failed to parse upgrade-path.json: %w", err)
	}

	return &spec, nil
}

func (p *OCIPuller) PullAndParseReleaseImage(ctx context.Context, image string) (*cfov1.ReleaseImageSpec, error) {
	dir, err := p.pullImage(ctx, image, "release")
	if err != nil {
		return nil, fmt.Errorf("failed to pull release image: %w", err)
	}

	releaseData, err := os.ReadFile(filepath.Join(dir, "release.json"))
	if err != nil {
		return nil, fmt.Errorf("failed to read release.json: %w", err)
	}

	var spec cfov1.ReleaseImageSpec
	if err := json.Unmarshal(releaseData, &spec); err != nil {
		return nil, fmt.Errorf("failed to parse release.json: %w", err)
	}

	return &spec, nil
}

func (p *OCIPuller) GetManifestDir(ctx context.Context, image string) (string, error) {
	dir, err := p.pullImage(ctx, image, "release")
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "manifests"), nil
}

func (p *OCIPuller) GetScriptsDir(ctx context.Context, image string) (string, error) {
	dir, err := p.pullImage(ctx, image, "release")
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "scripts"), nil
}

// GetImageDir returns the directory where the OCI image content is extracted.
func (p *OCIPuller) GetImageDir(ctx context.Context, image string) (string, error) {
	return p.pullImage(ctx, image, "release")
}

func (p *OCIPuller) pullImage(ctx context.Context, image, prefix string) (string, error) {
	dir := filepath.Join(p.workDir, prefix+"-"+safeImageName(image))

	if _, err := os.Stat(dir); err == nil {
		return dir, nil
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create work directory: %w", err)
	}

	fs, err := file.New(dir)
	if err != nil {
		return "", fmt.Errorf("failed to create file store: %w", err)
	}
	defer func() {
		// File store close errors are non-critical after successful copy
		_ = fs.Close()
	}()

	repo, err := remote.NewRepository(image)
	if err != nil {
		return "", fmt.Errorf("failed to parse image reference %s: %w", image, err)
	}

	if p.auth != nil && p.auth.Username != "" {
		repo.PlainHTTP = false
		repo.Client = &auth.Client{
			Client: retry.DefaultClient,
			Cache:  auth.NewCache(),
			Credential: auth.StaticCredential(repo.Reference.Registry, auth.Credential{
				Username: p.auth.Username,
				Password: p.auth.Password,
			}),
		}
	}

	_, err = oras.Copy(ctx, repo, repo.Reference.Reference, fs, repo.Reference.Reference, oras.DefaultCopyOptions)
	if err != nil {
		return "", fmt.Errorf("failed to pull OCI image %s: %w", image, err)
	}

	return dir, nil
}

func safeImageName(image string) string {
	result := ""
	for _, c := range image {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			result += string(c)
		} else {
			result += "-"
		}
	}
	return result
}
