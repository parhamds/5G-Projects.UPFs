// SPDX-License-Identifier: Apache-2.0
// Copyright 2020 Intel Corporation

package pfcpiface

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os/exec"
	"strings"

	log "github.com/sirupsen/logrus"
)

// NetworkSlice ... Config received for slice rates and DNN.
type NetworkSlice struct {
	SliceName string      `json:"sliceName"`
	SliceQos  SliceQos    `json:"sliceQos"`
	UeResInfo []UeResInfo `json:"ueResourceInfo"`
}

// SliceQos ... Slice level QOS rates.
type SliceQos struct {
	UplinkMbr    uint64 `json:"uplinkMbr"`
	DownlinkMbr  uint64 `json:"downlinkMbr"`
	BitrateUnit  string `json:"bitrateUnit"`
	UlBurstBytes uint64 `json:"uplinkBurstSize"`
	DlBurstBytes uint64 `json:"downlinkBurstSize"`
}

// UeResInfo ... UE Pool and DNN info.
type UeResInfo struct {
	Dnn  string `json:"dnn"`
	Name string `json:"uePoolId"`
}

type ConfigHandler struct {
	upf *upf
}
type RegisterGw struct {
	upf *upf
}

func setupConfigHandler(mux *http.ServeMux, upf *upf) {
	cfgHandler := ConfigHandler{upf: upf}
	mux.Handle("/v1/config/network-slices", &cfgHandler)
	registerGw := RegisterGw{upf: upf}
	mux.Handle("/registergw", &registerGw)
}

type GWRegisterReq struct {
	GwIP  string `json:"gwip"`
	GwMac string `json:"gwmac"`
}

func (registerGw *RegisterGw) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Infoln("handle http request for /registergw")

	switch r.Method {
	case "PUT":
		fallthrough
	case "POST":
		body, err := io.ReadAll(r.Body)
		if err != nil {
			log.Errorln("http req read body failed.")
			sendHTTPResp(http.StatusBadRequest, w)
		}

		log.Traceln(string(body))

		var registerReq GWRegisterReq

		err = json.Unmarshal(body, &registerReq)
		if err != nil {
			log.Errorln("Json unmarshal failed for http request")
			sendHTTPResp(http.StatusBadRequest, w)
		}

		err = registerGw.handleRegisterGW(registerReq)
		if err != nil {
			log.Errorln("handle gw register req failed")
			sendHTTPResp(http.StatusInternalServerError, w)
		}
		if registerReq.GwIP == registerGw.upf.gwIP {
			go PushPFCPInfoNew(registerGw.upf)
		}

		sendHTTPResp(http.StatusCreated, w)
	default:
		log.Infoln(w, "Sorry, only PUT and POST methods are supported.")
		sendHTTPResp(http.StatusMethodNotAllowed, w)
	}

}

func (registerGw *RegisterGw) handleRegisterGW(registerReq GWRegisterReq) error {

	var cmd *exec.Cmd

	reqGwOctets := strings.Split(registerReq.GwIP, ".")
	reqThirdOctet := reqGwOctets[2]

	accessGwOctets := strings.Split(registerGw.upf.AccessIP.String(), ".")
	accessThirdOctet := accessGwOctets[2]

	coreGwOctets := strings.Split(registerGw.upf.CoreIP.String(), ".")
	coreThirdOctet := coreGwOctets[2]

	var iface string

	switch reqThirdOctet {
	case accessThirdOctet:
		iface = "access"
	case coreThirdOctet:
		iface = "core"
	}
	accessGwip := fmt.Sprint("192.168.252.", reqGwOctets[3])
	coreGwip := fmt.Sprint("192.168.250.", reqGwOctets[3])
	addAccessRoute := exec.Command("ip", "route", "replace", "192.168.251.0/24", "via", accessGwip)
	fmt.Println(addAccessRoute.String())
	accesscombinedOutput, err := addAccessRoute.CombinedOutput()
	if err != nil {
		fmt.Printf("Error executing command: %v\nCombined Output: %s", cmd.String(), accesscombinedOutput)
		return err
	}

	addCoreRoute := exec.Command("ip", "route", "replace", "192.168.200.0/24", "via", coreGwip)
	corecombinedOutput, err := addCoreRoute.CombinedOutput()
	if err != nil {
		fmt.Printf("Error executing command: %v\nCombined Output: %s", cmd.String(), corecombinedOutput)
		return err
	}
	cmd = exec.Command("arp", "-s", registerReq.GwIP, registerReq.GwMac, "-i", iface)
	combinedOutput, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Error executing command: %v\nCombined Output: %s", cmd.String(), combinedOutput)
		return err
	}

	switch iface {
	case "access":
		registerGw.upf.accessGwRegistered = true
	case "core":
		registerGw.upf.coreGwRegistered = true
	}
	log.Traceln("static arp applied successfully for ip : ", registerReq.GwIP)
	return nil

}

func (c *ConfigHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Infoln("handle http request for /v1/config/network-slices")

	switch r.Method {
	case "PUT":
		fallthrough
	case "POST":
		body, err := io.ReadAll(r.Body)
		if err != nil {
			log.Errorln("http req read body failed.")
			sendHTTPResp(http.StatusBadRequest, w)
		}

		log.Traceln(string(body))

		var nwSlice NetworkSlice

		err = json.Unmarshal(body, &nwSlice)
		if err != nil {
			log.Errorln("Json unmarshal failed for http request")
			sendHTTPResp(http.StatusBadRequest, w)
		}

		handleSliceConfig(&nwSlice, c.upf)
		sendHTTPResp(http.StatusCreated, w)
	default:
		log.Infoln(w, "Sorry, only PUT and POST methods are supported.")
		sendHTTPResp(http.StatusMethodNotAllowed, w)
	}
}

func sendHTTPResp(status int, w http.ResponseWriter) {
	w.WriteHeader(status)
	w.Header().Set("Content-Type", "application/json")

	resp := make(map[string]string)

	switch status {
	case http.StatusCreated:
		resp["message"] = "Status Created"
	default:
		resp["message"] = "Failed to add slice"
	}

	jsonResp, err := json.Marshal(resp)
	if err != nil {
		log.Errorln("Error happened in JSON marshal. Err: ", err)
	}

	_, err = w.Write(jsonResp)
	if err != nil {
		log.Errorln("http response write failed : ", err)
	}
}

// calculateBitRates : Default bit rate is Mbps.
func calculateBitRates(mbr uint64, rate string) uint64 {
	var val int64

	switch rate {
	case "bps":
		return mbr
	case "Kbps":
		val = int64(mbr) * KB
	case "Gbps":
		val = int64(mbr) * GB
	case "Mbps":
		fallthrough
	default:
		val = int64(mbr) * MB
	}

	if val > 0 {
		return uint64(val)
	} else {
		return uint64(math.MaxInt64)
	}
}

func handleSliceConfig(nwSlice *NetworkSlice, upf *upf) {
	log.Infoln("handle slice config : ", nwSlice.SliceName)

	ulMbr := calculateBitRates(nwSlice.SliceQos.UplinkMbr,
		nwSlice.SliceQos.BitrateUnit)
	dlMbr := calculateBitRates(nwSlice.SliceQos.DownlinkMbr,
		nwSlice.SliceQos.BitrateUnit)
	sliceInfo := SliceInfo{
		name:         nwSlice.SliceName,
		uplinkMbr:    ulMbr,
		downlinkMbr:  dlMbr,
		ulBurstBytes: nwSlice.SliceQos.UlBurstBytes,
		dlBurstBytes: nwSlice.SliceQos.DlBurstBytes,
	}

	if len(nwSlice.UeResInfo) > 0 {
		sliceInfo.ueResList = make([]UeResource, 0)

		for _, ueRes := range nwSlice.UeResInfo {
			var ueResInfo UeResource
			ueResInfo.dnn = ueRes.Dnn
			ueResInfo.name = ueRes.Name
			sliceInfo.ueResList = append(sliceInfo.ueResList, ueResInfo)
		}
	}

	err := upf.addSliceInfo(&sliceInfo)
	if err != nil {
		log.Errorln("adding slice info to datapath failed : ", err)
	}
}
