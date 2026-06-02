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

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// UpgradeDuration tracks the duration of control plane upgrades.
	UpgradeDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "capbm_upgrade_duration_seconds",
			Help:    "Duration of control plane upgrade in seconds",
			Buckets: prometheus.ExponentialBuckets(60, 2, 10),
		},
		[]string{"cluster", "source_version", "target_version", "status"},
	)
	
	// UpgradeInProgress indicates whether an upgrade is currently in progress.
	UpgradeInProgress = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "capbm_upgrade_in_progress",
			Help: "Whether an upgrade is currently in progress",
		},
		[]string{"cluster"},
	)
	
	// NodeUpgradeDuration tracks the duration of single node upgrades.
	NodeUpgradeDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "capbm_node_upgrade_duration_seconds",
			Help:    "Duration of single node upgrade in seconds",
			Buckets: prometheus.ExponentialBuckets(30, 2, 8),
		},
		[]string{"cluster", "node", "component", "status"},
	)
	
	// EtcdBackupDuration tracks the duration of etcd backups.
	EtcdBackupDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "capbm_etcd_backup_duration_seconds",
			Help:    "Duration of etcd backup in seconds",
			Buckets: prometheus.ExponentialBuckets(10, 2, 6),
		},
		[]string{"cluster", "node", "status"},
	)
	
	// UpgradeErrorsTotal tracks the total number of upgrade errors.
	UpgradeErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "capbm_upgrade_errors_total",
			Help: "Total number of upgrade errors",
		},
		[]string{"cluster", "node", "component", "error_type"},
	)
	
	// UpgradeSessionsTotal tracks the total number of upgrade sessions.
	UpgradeSessionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "capbm_upgrade_sessions_total",
			Help: "Total number of upgrade sessions",
		},
		[]string{"cluster", "status"},
	)
)

func init() {
	// Register metrics with the controller-runtime metrics registry
	metrics.Registry.MustRegister(
		UpgradeDuration,
		UpgradeInProgress,
		NodeUpgradeDuration,
		EtcdBackupDuration,
		UpgradeErrorsTotal,
		UpgradeSessionsTotal,
	)
}
