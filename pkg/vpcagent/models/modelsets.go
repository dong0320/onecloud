// Copyright 2019 Yunion
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package models

import (
	"strings"
	"time"

	"yunion.io/x/onecloud/pkg/vpcagent/apihelper"
)

// pluralMap maps from KeyPlurals to underscore-separated field names
var pluralMap = map[string]string{}

func init() {
	// XXX drop this
	ss := []string{
		"vpcs",
		"networks",
		"guestnetworks",
	}
	for _, s := range ss {
		k := strings.Replace(s, "_", "", -1)
		pluralMap[k] = s
	}
}

type ModelSetsMaxUpdatedAt struct {
	Vpcs               time.Time
	Wires              time.Time
	Networks           time.Time
	Guests             time.Time
	Hosts              time.Time
	SecurityGroups     time.Time
	SecurityGroupRules time.Time
	Guestnetworks      time.Time
	Guestsecgroups     time.Time
}

func NewModelSetsMaxUpdatedAt() *ModelSetsMaxUpdatedAt {
	return &ModelSetsMaxUpdatedAt{
		Vpcs:               apihelper.PseudoZeroTime,
		Wires:              apihelper.PseudoZeroTime,
		Networks:           apihelper.PseudoZeroTime,
		Guests:             apihelper.PseudoZeroTime,
		Hosts:              apihelper.PseudoZeroTime,
		SecurityGroups:     apihelper.PseudoZeroTime,
		SecurityGroupRules: apihelper.PseudoZeroTime,
		Guestnetworks:      apihelper.PseudoZeroTime,
		Guestsecgroups:     apihelper.PseudoZeroTime,
	}
}

type ModelSets struct {
	Vpcs               Vpcs
	Wires              Wires
	Networks           Networks
	Guests             Guests
	Hosts              Hosts
	SecurityGroups     SecurityGroups
	SecurityGroupRules SecurityGroupRules
	Guestnetworks      Guestnetworks
	Guestsecgroups     Guestsecgroups
}

func NewModelSets() *ModelSets {
	return &ModelSets{
		Vpcs:               Vpcs{},
		Wires:              Wires{},
		Networks:           Networks{},
		Guests:             Guests{},
		Hosts:              Hosts{},
		SecurityGroups:     SecurityGroups{},
		SecurityGroupRules: SecurityGroupRules{},
		Guestnetworks:      Guestnetworks{},
		Guestsecgroups:     Guestsecgroups{},
	}
}

func (mss *ModelSets) ModelSetList() []apihelper.IModelSet {
	// it's ordered this way to favour creation, not deletion
	return []apihelper.IModelSet{
		mss.Vpcs,
		mss.Wires,
		mss.Networks,
		mss.Guests,
		mss.Hosts,
		mss.SecurityGroups,
		mss.SecurityGroupRules,
		mss.Guestnetworks,
		mss.Guestsecgroups,
	}
}

func (mss *ModelSets) NewEmpty() apihelper.IModelSets {
	return NewModelSets()
}

func (mss *ModelSets) Copy() apihelper.IModelSets {
	mssCopy := &ModelSets{
		Vpcs:               mss.Vpcs.Copy().(Vpcs),
		Wires:              mss.Wires.Copy().(Wires),
		Networks:           mss.Networks.Copy().(Networks),
		Guests:             mss.Guests.Copy().(Guests),
		Hosts:              mss.Hosts.Copy().(Hosts),
		SecurityGroups:     mss.SecurityGroups.Copy().(SecurityGroups),
		SecurityGroupRules: mss.SecurityGroupRules.Copy().(SecurityGroupRules),
		Guestnetworks:      mss.Guestnetworks.Copy().(Guestnetworks),
		Guestsecgroups:     mss.Guestsecgroups.Copy().(Guestsecgroups),
	}
	mssCopy.join()
	return mssCopy
}

func (mss *ModelSets) ApplyUpdates(mssNews apihelper.IModelSets) apihelper.ModelSetsUpdateResult {
	r := apihelper.ModelSetsUpdateResult{
		Changed: false,
		Correct: true,
	}
	mssList := mss.ModelSetList()
	mssNewsList := mssNews.ModelSetList()
	for i, mss := range mssList {
		mssNews := mssNewsList[i]
		msR := apihelper.ModelSetApplyUpdates(mss, mssNews)
		if !r.Changed && msR.Changed {
			r.Changed = true
		}
	}
	if r.Changed {
		r.Correct = mss.join()
	}
	return r
}

func (mss *ModelSets) join() bool {
	mss.Guests.initJoin()
	var p []bool
	p = append(p, mss.Vpcs.joinWires(mss.Wires))
	p = append(p, mss.Wires.joinNetworks(mss.Networks))
	p = append(p, mss.Vpcs.joinNetworks(mss.Networks))
	p = append(p, mss.Networks.joinGuestnetworks(mss.Guestnetworks))
	p = append(p, mss.Guests.joinHosts(mss.Hosts))
	p = append(p, mss.Guests.joinSecurityGroups(mss.SecurityGroups))
	p = append(p, mss.SecurityGroups.joinSecurityGroupRules(mss.SecurityGroupRules))
	p = append(p, mss.Guestsecgroups.join(mss.SecurityGroups, mss.Guests))
	p = append(p, mss.Guestnetworks.joinGuests(mss.Guests))
	for _, b := range p {
		if !b {
			return false
		}
	}
	return true
}