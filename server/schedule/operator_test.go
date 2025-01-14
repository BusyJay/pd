// Copyright 2016 PingCAP, Inc.
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

package schedule

import (
	"encoding/json"
	"sync/atomic"
	"time"

	. "github.com/pingcap/check"
	"github.com/pingcap/kvproto/pkg/metapb"
	"github.com/pingcap/pd/server/core"
)

var _ = Suite(&testOperatorSuite{})

type testOperatorSuite struct{}

func (s *testOperatorSuite) newTestRegion(regionID uint64, leaderPeer uint64, peers ...[2]uint64) *core.RegionInfo {
	var (
		region metapb.Region
		leader *metapb.Peer
	)
	region.Id = regionID
	for i := range peers {
		peer := &metapb.Peer{
			Id:      peers[i][1],
			StoreId: peers[i][0],
		}
		region.Peers = append(region.Peers, peer)
		if peer.GetId() == leaderPeer {
			leader = peer
		}
	}
	regionInfo := core.NewRegionInfo(&region, leader, core.SetApproximateSize(10), core.SetApproximateKeys(10))
	return regionInfo
}

func (s *testOperatorSuite) TestOperatorStep(c *C) {
	region := s.newTestRegion(1, 1, [2]uint64{1, 1}, [2]uint64{2, 2})
	c.Assert(TransferLeader{FromStore: 1, ToStore: 2}.IsFinish(region), IsFalse)
	c.Assert(TransferLeader{FromStore: 2, ToStore: 1}.IsFinish(region), IsTrue)
	c.Assert(AddPeer{ToStore: 3, PeerID: 3}.IsFinish(region), IsFalse)
	c.Assert(AddPeer{ToStore: 1, PeerID: 1}.IsFinish(region), IsTrue)
	c.Assert(RemovePeer{FromStore: 1}.IsFinish(region), IsFalse)
	c.Assert(RemovePeer{FromStore: 3}.IsFinish(region), IsTrue)
}

func (s *testOperatorSuite) newTestOperator(regionID uint64, kind OperatorKind, steps ...OperatorStep) *Operator {
	return NewOperator("test", regionID, &metapb.RegionEpoch{}, OpAdmin|kind, steps...)
}

func (s *testOperatorSuite) checkSteps(c *C, op *Operator, steps []OperatorStep) {
	c.Assert(op.Len(), Equals, len(steps))
	for i := range steps {
		c.Assert(op.Step(i), Equals, steps[i])
	}
}

func (s *testOperatorSuite) TestOperator(c *C) {
	region := s.newTestRegion(1, 1, [2]uint64{1, 1}, [2]uint64{2, 2})
	// addPeer1, transferLeader1, removePeer3
	steps := []OperatorStep{
		AddPeer{ToStore: 1, PeerID: 1},
		TransferLeader{FromStore: 3, ToStore: 1},
		RemovePeer{FromStore: 3},
	}
	op := s.newTestOperator(1, OpLeader|OpRegion, steps...)
	s.checkSteps(c, op, steps)
	c.Assert(op.Check(region), IsNil)
	c.Assert(op.IsFinish(), IsTrue)
	op.startTime = time.Now()
	op.startTime = op.startTime.Add(-RegionOperatorWaitTime - time.Second)
	c.Assert(op.IsTimeout(), IsFalse)

	// addPeer1, transferLeader1, removePeer2
	steps = []OperatorStep{
		AddPeer{ToStore: 1, PeerID: 1},
		TransferLeader{FromStore: 2, ToStore: 1},
		RemovePeer{FromStore: 2},
	}
	op = s.newTestOperator(1, OpLeader|OpRegion, steps...)
	s.checkSteps(c, op, steps)
	c.Assert(op.Check(region), Equals, RemovePeer{FromStore: 2})
	c.Assert(atomic.LoadInt32(&op.currentStep), Equals, int32(2))
	op.startTime = time.Now()
	c.Assert(op.IsTimeout(), IsFalse)
	op.startTime = op.startTime.Add(-LeaderOperatorWaitTime - time.Second)
	c.Assert(op.IsTimeout(), IsFalse)
	op.startTime = op.startTime.Add(-RegionOperatorWaitTime - time.Second)
	c.Assert(op.IsTimeout(), IsTrue)
	res, err := json.Marshal(op)
	c.Assert(err, IsNil)
	c.Assert(len(res), Equals, len(op.String())+2)

	// check short timeout for transfer leader only operators.
	steps = []OperatorStep{TransferLeader{FromStore: 2, ToStore: 1}}
	op = s.newTestOperator(1, OpLeader, steps...)
	op.startTime = time.Now()
	c.Assert(op.IsTimeout(), IsFalse)
	op.startTime = op.startTime.Add(-LeaderOperatorWaitTime - time.Second)
	c.Assert(op.IsTimeout(), IsTrue)
}

func (s *testOperatorSuite) TestInfluence(c *C) {
	region := s.newTestRegion(1, 1, [2]uint64{1, 1}, [2]uint64{2, 2})
	opInfluence := OpInfluence{storesInfluence: make(map[uint64]*StoreInfluence)}
	storeOpInfluence := opInfluence.storesInfluence
	storeOpInfluence[1] = &StoreInfluence{}
	storeOpInfluence[2] = &StoreInfluence{}

	AddPeer{ToStore: 2, PeerID: 2}.Influence(opInfluence, region)
	c.Assert(*storeOpInfluence[2], DeepEquals, StoreInfluence{
		LeaderSize:  0,
		LeaderCount: 0,
		RegionSize:  10,
		RegionCount: 1,
		StepCost:    1000,
	})

	TransferLeader{FromStore: 1, ToStore: 2}.Influence(opInfluence, region)
	c.Assert(*storeOpInfluence[1], DeepEquals, StoreInfluence{
		LeaderSize:  -10,
		LeaderCount: -1,
		RegionSize:  0,
		RegionCount: 0,
		StepCost:    0,
	})
	c.Assert(*storeOpInfluence[2], DeepEquals, StoreInfluence{
		LeaderSize:  10,
		LeaderCount: 1,
		RegionSize:  10,
		RegionCount: 1,
		StepCost:    1000,
	})

	RemovePeer{FromStore: 1}.Influence(opInfluence, region)
	c.Assert(*storeOpInfluence[1], DeepEquals, StoreInfluence{
		LeaderSize:  -10,
		LeaderCount: -1,
		RegionSize:  -10,
		RegionCount: -1,
		StepCost:    0,
	})
	c.Assert(*storeOpInfluence[2], DeepEquals, StoreInfluence{
		LeaderSize:  10,
		LeaderCount: 1,
		RegionSize:  10,
		RegionCount: 1,
		StepCost:    1000,
	})

	MergeRegion{IsPassive: false}.Influence(opInfluence, region)
	c.Assert(*storeOpInfluence[1], DeepEquals, StoreInfluence{
		LeaderSize:  -10,
		LeaderCount: -1,
		RegionSize:  -10,
		RegionCount: -1,
		StepCost:    0,
	})
	c.Assert(*storeOpInfluence[2], DeepEquals, StoreInfluence{
		LeaderSize:  10,
		LeaderCount: 1,
		RegionSize:  10,
		RegionCount: 1,
		StepCost:    1000,
	})

	MergeRegion{IsPassive: true}.Influence(opInfluence, region)
	c.Assert(*storeOpInfluence[1], DeepEquals, StoreInfluence{
		LeaderSize:  -10,
		LeaderCount: -2,
		RegionSize:  -10,
		RegionCount: -2,
		StepCost:    0,
	})
	c.Assert(*storeOpInfluence[2], DeepEquals, StoreInfluence{
		LeaderSize:  10,
		LeaderCount: 1,
		RegionSize:  10,
		RegionCount: 0,
		StepCost:    1000,
	})
}

func (s *testOperatorSuite) TestOperatorKind(c *C) {
	c.Assert((OpLeader | OpReplica).String(), Equals, "leader,replica")
	c.Assert(OperatorKind(0).String(), Equals, "unknown")
	k, err := ParseOperatorKind("balance,region,leader")
	c.Assert(err, IsNil)
	c.Assert(k, Equals, OpBalance|OpRegion|OpLeader)
	_, err = ParseOperatorKind("leader,region")
	c.Assert(err, IsNil)
	_, err = ParseOperatorKind("foobar")
	c.Assert(err, NotNil)
}
