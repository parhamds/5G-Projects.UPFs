// SPDX-License-Identifier: Apache-2.0
// Copyright 2022-present Open Networking Foundation

package pfcpiface

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

type InMemoryStore struct {
	// sessions stores all PFCP sessions.
	// sync.Map is optimized for case when multiple goroutines
	// read, write, and overwrite entries for disjoint sets of keys.
	sessions sync.Map
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{}
}

func (i *InMemoryStore) GetAllSessions() []PFCPSession {
	sessions := make([]PFCPSession, 0)

	i.sessions.Range(func(key, value interface{}) bool {
		v := value.(PFCPSession)
		sessions = append(sessions, v)
		return true
	})

	log.WithFields(log.Fields{
		"sessions": sessions,
	}).Trace("Got all PFCP sessions from local store")

	return sessions
}

type RuleReq struct {
	Teid []string `json:"teid"`
	Ip   []string `json:"ip"`
}

func PushPDRInfo(addresses []uint32) {
	addrStr := make([]string, 0)
	for _, i := range addresses {
		ipStr := int2ip(i)
		addrStr = append(addrStr, ipStr.String())
	}
	rulereq := RuleReq{
		Ip: addrStr,
	}
	ruleReqJson, _ := json.Marshal(rulereq)

	fmt.Printf("parham log : json encoded pfcpInfo [%s] ", ruleReqJson)

	// change the IP here
	requestURL := "http://exitlb:8080/addrule"
	jsonBody := []byte(ruleReqJson)

	bodyReader := bytes.NewReader(jsonBody)
	req, err := http.NewRequest(http.MethodPost, requestURL, bodyReader)
	if err != nil {
		log.Errorf("client: could not create request: %s\n", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := http.Client{
		Timeout: 10 * time.Second,
	}
	done := false
	for !done {
		resp, err := client.Do(req)
		if err != nil {
			log.Errorf("client: error making http request: %s\n", err)
		} else if resp.StatusCode == http.StatusCreated {
			done = true
			fmt.Println("parham log : resp header = ", resp.Header)
			fmt.Println("parham log : resp status = ", resp.Status)
			return
		}
		time.Sleep(1 * time.Second)
	}

}

func (i *InMemoryStore) PutSession(session PFCPSession) error {
	if session.localSEID == 0 {
		return ErrInvalidArgument("session.localSEID", session.localSEID)
	}

	i.sessions.Store(session.localSEID, session)

	log.WithFields(log.Fields{
		"session": session,
	}).Trace("Saved PFCP sessions to local store")
	uEAddresses := make([]uint32, 0)
	for _, v := range session.pdrs {
		exists := false
		for _, u := range uEAddresses {
			if u == v.ueAddress {
				exists = true
				break
			}
		}
		if !exists {
			uEAddresses = append(uEAddresses, v.ueAddress)
		}
	}
	go PushPDRInfo(uEAddresses)
	return nil
}

func (i *InMemoryStore) DeleteSession(fseid uint64) error {
	i.sessions.Delete(fseid)

	log.WithFields(log.Fields{
		"F-SEID": fseid,
	}).Trace("PFCP session removed from local store")

	return nil
}

func (i *InMemoryStore) DeleteAllSessions() bool {
	i.sessions.Range(func(key, value interface{}) bool {
		i.sessions.Delete(key)
		return true
	})

	log.Trace("All PFCP sessions removed from local store")

	return true
}

func (i *InMemoryStore) GetSession(fseid uint64) (PFCPSession, bool) {
	sess, ok := i.sessions.Load(fseid)
	if !ok {
		return PFCPSession{}, false
	}

	session, ok := sess.(PFCPSession)
	if !ok {
		return PFCPSession{}, false
	}

	log.WithFields(log.Fields{
		"session": session,
	}).Trace("Got PFCP session from local store")

	return session, ok
}
