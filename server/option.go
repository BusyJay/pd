// Copyright 2017 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coreos/go-semver/semver"
	"github.com/pingcap/kvproto/pkg/metapb"
	"github.com/pingcap/pd/server/core"
	"github.com/pingcap/pd/server/schedule"
)

// scheduleOption is a wrapper to access the configuration safely.
type scheduleOption struct {
	v              atomic.Value
	rep            *Replication
	ns             sync.Map // concurrent map[string]*namespaceOption
	labelProperty  atomic.Value
	clusterVersion atomic.Value
	pdServerConfig atomic.Value
}

func newScheduleOption(cfg *Config) *scheduleOption {
	o := &scheduleOption{}
	o.store(&cfg.Schedule)
	o.ns = sync.Map{}
	for name, nsCfg := range cfg.Namespace {
		nsCfg := nsCfg
		o.ns.Store(name, newNamespaceOption(&nsCfg))
	}
	o.rep = newReplication(&cfg.Replication)
	o.pdServerConfig.Store(&cfg.PDServerCfg)
	o.labelProperty.Store(cfg.LabelProperty)
	o.clusterVersion.Store(cfg.ClusterVersion)
	return o
}

func (o *scheduleOption) load() *ScheduleConfig {
	return o.v.Load().(*ScheduleConfig)
}

func (o *scheduleOption) store(cfg *ScheduleConfig) {
	o.v.Store(cfg)
}

func (o *scheduleOption) GetReplication() *Replication {
	return o.rep
}

func (o *scheduleOption) getNS(name string) (*namespaceOption, bool) {
	if n, ok := o.ns.Load(name); ok {
		if n, ok := n.(*namespaceOption); ok {
			return n, true
		}
	}
	return nil, false
}

func (o *scheduleOption) loadNSConfig() map[string]NamespaceConfig {
	namespaces := make(map[string]NamespaceConfig)
	f := func(k, v interface{}) bool {
		var kstr string
		var ok bool
		if kstr, ok = k.(string); !ok {
			return false
		}
		if ns, ok := v.(*namespaceOption); ok {
			namespaces[kstr] = *ns.load()
			return true
		}
		return false
	}
	o.ns.Range(f)

	return namespaces
}

func (o *scheduleOption) GetMaxReplicas(name string) int {
	if n, ok := o.getNS(name); ok {
		return n.GetMaxReplicas()
	}
	return o.rep.GetMaxReplicas()
}

func (o *scheduleOption) SetMaxReplicas(replicas int) {
	o.rep.SetMaxReplicas(replicas)
}

func (o *scheduleOption) GetLocationLabels() []string {
	return o.rep.GetLocationLabels()
}

func (o *scheduleOption) GetMaxSnapshotCount() uint64 {
	return o.load().MaxSnapshotCount
}

func (o *scheduleOption) GetMaxPendingPeerCount() uint64 {
	return o.load().MaxPendingPeerCount
}

func (o *scheduleOption) GetMaxMergeRegionSize() uint64 {
	return o.load().MaxMergeRegionSize
}

func (o *scheduleOption) GetMaxMergeRegionKeys() uint64 {
	return o.load().MaxMergeRegionKeys
}

func (o *scheduleOption) GetSplitMergeInterval() time.Duration {
	return o.load().SplitMergeInterval.Duration
}

func (o *scheduleOption) GetEnableOneWayMerge() bool {
	return o.load().EnableOneWayMerge
}

func (o *scheduleOption) GetPatrolRegionInterval() time.Duration {
	return o.load().PatrolRegionInterval.Duration
}

func (o *scheduleOption) GetMaxStoreDownTime() time.Duration {
	return o.load().MaxStoreDownTime.Duration
}

func (o *scheduleOption) GetLeaderScheduleLimit(name string) uint64 {
	if n, ok := o.getNS(name); ok {
		return n.GetLeaderScheduleLimit()
	}
	return o.load().LeaderScheduleLimit
}

func (o *scheduleOption) GetRegionScheduleLimit(name string) uint64 {
	if n, ok := o.getNS(name); ok {
		return n.GetRegionScheduleLimit()
	}
	return o.load().RegionScheduleLimit
}

func (o *scheduleOption) GetReplicaScheduleLimit(name string) uint64 {
	if n, ok := o.getNS(name); ok {
		return n.GetReplicaScheduleLimit()
	}
	return o.load().ReplicaScheduleLimit
}

func (o *scheduleOption) GetMergeScheduleLimit(name string) uint64 {
	if n, ok := o.getNS(name); ok {
		return n.GetMergeScheduleLimit()
	}
	return o.load().MergeScheduleLimit
}

func (o *scheduleOption) GetHotRegionScheduleLimit(name string) uint64 {
	if n, ok := o.getNS(name); ok {
		return n.GetHotRegionScheduleLimit()
	}
	return o.load().HotRegionScheduleLimit
}

func (o *scheduleOption) GetStoreBalanceRate() float64 {
	return o.load().StoreBalanceRate
}

func (o *scheduleOption) GetTolerantSizeRatio() float64 {
	return o.load().TolerantSizeRatio
}

func (o *scheduleOption) GetLowSpaceRatio() float64 {
	return o.load().LowSpaceRatio
}

func (o *scheduleOption) GetHighSpaceRatio() float64 {
	return o.load().HighSpaceRatio
}

func (o *scheduleOption) GetSchedulerMaxWaitingOperator() uint64 {
	return o.load().SchedulerMaxWaitingOperator
}

func (o *scheduleOption) IsRaftLearnerEnabled() bool {
	return !o.load().DisableLearner
}

func (o *scheduleOption) IsRemoveDownReplicaEnabled() bool {
	return !o.load().DisableRemoveDownReplica
}

func (o *scheduleOption) IsReplaceOfflineReplicaEnabled() bool {
	return !o.load().DisableReplaceOfflineReplica
}

func (o *scheduleOption) IsMakeUpReplicaEnabled() bool {
	return !o.load().DisableMakeUpReplica
}

func (o *scheduleOption) IsRemoveExtraReplicaEnabled() bool {
	return !o.load().DisableRemoveExtraReplica
}

func (o *scheduleOption) IsLocationReplacementEnabled() bool {
	return !o.load().DisableLocationReplacement
}

func (o *scheduleOption) IsNamespaceRelocationEnabled() bool {
	return !o.load().DisableNamespaceRelocation
}

func (o *scheduleOption) GetSchedulers() SchedulerConfigs {
	return o.load().Schedulers
}

func (o *scheduleOption) AddSchedulerCfg(tp string, args []string) {
	c := o.load()
	v := c.clone()
	for i, schedulerCfg := range v.Schedulers {
		// comparing args is to cover the case that there are schedulers in same type but not with same name
		// such as two schedulers of type "evict-leader",
		// one name is "evict-leader-scheduler-1" and the other is "evict-leader-scheduler-2"
		if reflect.DeepEqual(schedulerCfg, SchedulerConfig{Type: tp, Args: args, Disable: false}) {
			return
		}

		if reflect.DeepEqual(schedulerCfg, SchedulerConfig{Type: tp, Args: args, Disable: true}) {
			schedulerCfg.Disable = false
			v.Schedulers[i] = schedulerCfg
			o.store(v)
			return
		}
	}
	v.Schedulers = append(v.Schedulers, SchedulerConfig{Type: tp, Args: args, Disable: false})
	o.store(v)
}

func (o *scheduleOption) RemoveSchedulerCfg(name string) error {
	c := o.load()
	v := c.clone()
	for i, schedulerCfg := range v.Schedulers {
		// To create a temporary scheduler is just used to get scheduler's name
		tmp, err := schedule.CreateScheduler(schedulerCfg.Type, schedule.NewOperatorController(nil, nil), schedulerCfg.Args...)
		if err != nil {
			return err
		}
		if tmp.GetName() == name {
			if IsDefaultScheduler(tmp.GetType()) {
				schedulerCfg.Disable = true
				v.Schedulers[i] = schedulerCfg
			} else {
				v.Schedulers = append(v.Schedulers[:i], v.Schedulers[i+1:]...)
			}
			o.store(v)
			return nil
		}
	}
	return nil
}

func (o *scheduleOption) SetLabelProperty(typ, labelKey, labelValue string) {
	cfg := o.loadLabelPropertyConfig().clone()
	for _, l := range cfg[typ] {
		if l.Key == labelKey && l.Value == labelValue {
			return
		}
	}
	cfg[typ] = append(cfg[typ], StoreLabel{Key: labelKey, Value: labelValue})
	o.labelProperty.Store(cfg)
}

func (o *scheduleOption) DeleteLabelProperty(typ, labelKey, labelValue string) {
	cfg := o.loadLabelPropertyConfig().clone()
	oldLabels := cfg[typ]
	cfg[typ] = []StoreLabel{}
	for _, l := range oldLabels {
		if l.Key == labelKey && l.Value == labelValue {
			continue
		}
		cfg[typ] = append(cfg[typ], l)
	}
	if len(cfg[typ]) == 0 {
		delete(cfg, typ)
	}
	o.labelProperty.Store(cfg)
}

func (o *scheduleOption) loadLabelPropertyConfig() LabelPropertyConfig {
	return o.labelProperty.Load().(LabelPropertyConfig)
}

func (o *scheduleOption) SetClusterVersion(v semver.Version) {
	o.clusterVersion.Store(v)
}

func (o *scheduleOption) loadClusterVersion() semver.Version {
	return o.clusterVersion.Load().(semver.Version)
}

func (o *scheduleOption) loadPDServerConfig() *PDServerConfig {
	return o.pdServerConfig.Load().(*PDServerConfig)
}

func (o *scheduleOption) persist(kv *core.KV) error {
	namespaces := o.loadNSConfig()

	cfg := &Config{
		Schedule:       *o.load(),
		Replication:    *o.rep.load(),
		Namespace:      namespaces,
		LabelProperty:  o.loadLabelPropertyConfig(),
		ClusterVersion: o.loadClusterVersion(),
		PDServerCfg:    *o.loadPDServerConfig(),
	}
	err := kv.SaveConfig(cfg)
	return err
}

func (o *scheduleOption) reload(kv *core.KV) error {
	namespaces := o.loadNSConfig()

	cfg := &Config{
		Schedule:       *o.load().clone(),
		Replication:    *o.rep.load(),
		Namespace:      namespaces,
		LabelProperty:  o.loadLabelPropertyConfig().clone(),
		ClusterVersion: o.loadClusterVersion(),
		PDServerCfg:    *o.loadPDServerConfig(),
	}
	isExist, err := kv.LoadConfig(cfg)
	if err != nil {
		return err
	}
	o.adjustScheduleCfg(cfg)
	if isExist {
		o.store(&cfg.Schedule)
		o.rep.store(&cfg.Replication)
		for name, nsCfg := range cfg.Namespace {
			nsCfg := nsCfg
			o.ns.Store(name, newNamespaceOption(&nsCfg))
		}
		o.labelProperty.Store(cfg.LabelProperty)
		o.clusterVersion.Store(cfg.ClusterVersion)
		o.pdServerConfig.Store(&cfg.PDServerCfg)
	}
	return nil
}

func (o *scheduleOption) adjustScheduleCfg(persistentCfg *Config) {
	scheduleCfg := o.load().clone()
	for i, s := range scheduleCfg.Schedulers {
		for _, ps := range persistentCfg.Schedule.Schedulers {
			if s.Type == ps.Type && reflect.DeepEqual(s.Args, ps.Args) {
				scheduleCfg.Schedulers[i].Disable = ps.Disable
				break
			}
		}
	}
	restoredSchedulers := make([]SchedulerConfig, 0, len(persistentCfg.Schedule.Schedulers))
	for _, ps := range persistentCfg.Schedule.Schedulers {
		needRestore := true
		for _, s := range scheduleCfg.Schedulers {
			if s.Type == ps.Type && reflect.DeepEqual(s.Args, ps.Args) {
				needRestore = false
				break
			}
		}
		if needRestore {
			restoredSchedulers = append(restoredSchedulers, ps)
		}
	}
	scheduleCfg.Schedulers = append(scheduleCfg.Schedulers, restoredSchedulers...)
	persistentCfg.Schedule.Schedulers = scheduleCfg.Schedulers
	o.store(scheduleCfg)
}

func (o *scheduleOption) GetHotRegionCacheHitsThreshold() int {
	return int(o.load().HotRegionCacheHitsThreshold)
}

func (o *scheduleOption) CheckLabelProperty(typ string, labels []*metapb.StoreLabel) bool {
	pc := o.labelProperty.Load().(LabelPropertyConfig)
	for _, cfg := range pc[typ] {
		for _, l := range labels {
			if l.Key == cfg.Key && l.Value == cfg.Value {
				return true
			}
		}
	}
	return false
}

// Replication provides some help to do replication.
type Replication struct {
	replicateCfg atomic.Value
}

func newReplication(cfg *ReplicationConfig) *Replication {
	r := &Replication{}
	r.store(cfg)
	return r
}

func (r *Replication) load() *ReplicationConfig {
	return r.replicateCfg.Load().(*ReplicationConfig)
}

func (r *Replication) store(cfg *ReplicationConfig) {
	r.replicateCfg.Store(cfg)
}

// GetMaxReplicas returns the number of replicas for each region.
func (r *Replication) GetMaxReplicas() int {
	return int(r.load().MaxReplicas)
}

// SetMaxReplicas set the replicas for each region.
func (r *Replication) SetMaxReplicas(replicas int) {
	c := r.load()
	v := c.clone()
	v.MaxReplicas = uint64(replicas)
	r.store(v)
}

// GetLocationLabels returns the location labels for each region
func (r *Replication) GetLocationLabels() []string {
	return r.load().LocationLabels
}

// GetStrictlyMatchLabel returns whether check label strict.
func (r *Replication) GetStrictlyMatchLabel() bool {
	return r.load().StrictlyMatchLabel
}

// namespaceOption is a wrapper to access the configuration safely.
type namespaceOption struct {
	namespaceCfg atomic.Value
}

func newNamespaceOption(cfg *NamespaceConfig) *namespaceOption {
	n := &namespaceOption{}
	n.store(cfg)
	return n
}

func (n *namespaceOption) load() *NamespaceConfig {
	return n.namespaceCfg.Load().(*NamespaceConfig)
}

func (n *namespaceOption) store(cfg *NamespaceConfig) {
	n.namespaceCfg.Store(cfg)
}

// GetMaxReplicas returns the number of replicas for each region.
func (n *namespaceOption) GetMaxReplicas() int {
	return int(n.load().MaxReplicas)
}

// GetLeaderScheduleLimit returns the limit for leader schedule.
func (n *namespaceOption) GetLeaderScheduleLimit() uint64 {
	return n.load().LeaderScheduleLimit
}

// GetRegionScheduleLimit returns the limit for region schedule.
func (n *namespaceOption) GetRegionScheduleLimit() uint64 {
	return n.load().RegionScheduleLimit
}

// GetReplicaScheduleLimit returns the limit for replica schedule.
func (n *namespaceOption) GetReplicaScheduleLimit() uint64 {
	return n.load().ReplicaScheduleLimit
}

// GetMergeScheduleLimit returns the limit for merge schedule.
func (n *namespaceOption) GetMergeScheduleLimit() uint64 {
	return n.load().MergeScheduleLimit
}

// GetHotRegionScheduleLimit returns the limit for hot region schedule.
func (n *namespaceOption) GetHotRegionScheduleLimit() uint64 {
	return n.load().HotRegionScheduleLimit
}
