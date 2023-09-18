// SPDX-License-Identifier: Apache-2.0
// Copyright 2020 Intel Corporation

package pfcpiface

import (
	"fmt"

	log "github.com/sirupsen/logrus"

	"github.com/omec-project/upf-epc/pfcpiface/metrics"
)

type PacketForwardingRules struct {
	pdrs []pdr
	fars []far
	qers []qer
}

// PFCPSession implements one PFCP session.
type PFCPSession struct {
	localSEID  uint64
	remoteSEID uint64
	metrics    *metrics.Session
	PacketForwardingRules
}

func (p PacketForwardingRules) String() string {
	return fmt.Sprintf("PDRs=%v, FARs=%v, QERs=%v", p.pdrs, p.fars, p.qers)
}

// NewPFCPSession allocates an session with ID.
func (pConn *PFCPConn) NewPFCPSession(rseid uint64) (PFCPSession, bool) {

	s := PFCPSession{
		localSEID:  rseid,
		remoteSEID: rseid,
		PacketForwardingRules: PacketForwardingRules{
			pdrs: make([]pdr, 0, MaxItems),
			fars: make([]far, 0, MaxItems),
			qers: make([]qer, 0, MaxItems),
		},
	}
	s.metrics = metrics.NewSession(pConn.nodeID.remote)

	// Metrics update
	pConn.SaveSessions(s.metrics)

	return s, true

}

// RemoveSession removes session using lseid.
func (pConn *PFCPConn) RemoveSession(session PFCPSession) {
	// Metrics update
	session.metrics.Delete()
	pConn.SaveSessions(session.metrics)

	if err := pConn.store.DeleteSession(session.localSEID); err != nil {
		log.Errorf("Failed to delete PFCP session from store: %v", err)
	}
}
