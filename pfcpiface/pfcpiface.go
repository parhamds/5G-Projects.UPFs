// SPDX-License-Identifier: Apache-2.0
// Copyright 2022-present Open Networking Foundation

package pfcpiface

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	reuse "github.com/libp2p/go-reuseport"
	log "github.com/sirupsen/logrus"
)

var (
	simulate = simModeDisable
)

func init() {
	flag.Var(&simulate, "simulate", "create|delete|create_continue simulated sessions")
}

type PFCPIface struct {
	conf Conf

	node *PFCPNode
	fp   datapath
	upf  *upf

	httpSrv      *http.Server
	httpEndpoint string

	uc *upfCollector
	nc *PfcpNodeCollector

	mu sync.Mutex
}

func NewPFCPIface(conf Conf) *PFCPIface {
	pfcpIface := &PFCPIface{
		conf: conf,
	}

	if conf.EnableP4rt {
		pfcpIface.fp = &UP4{}
	} else {
		pfcpIface.fp = &bess{}
	}

	httpPort := "8080"
	if conf.CPIface.HTTPPort != "" {
		httpPort = conf.CPIface.HTTPPort
	}

	pfcpIface.httpEndpoint = ":" + httpPort

	pfcpIface.upf = NewUPF(&conf, pfcpIface.fp)

	return pfcpIface
}

func (p *PFCPIface) mustInit() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.node = NewPFCPNode(p.upf, &p.conf)
	httpMux := http.NewServeMux()

	setupConfigHandler(httpMux, p.upf)

	var err error

	p.uc, p.nc, err = setupProm(httpMux, p.upf, p.node)

	if err != nil {
		log.Fatalln("setupProm failed", err)
	}

	// Note: due to error with golangci-lint ("Error: G112: Potential Slowloris Attack
	// because ReadHeaderTimeout is not configured in the http.Server (gosec)"),
	// the ReadHeaderTimeout is set to the same value as in nginx (client_header_timeout)
	p.httpSrv = &http.Server{Addr: p.httpEndpoint, Handler: httpMux, ReadHeaderTimeout: 60 * time.Second}
}

func (p *PFCPIface) Run() {
	if simulate.enable() {
		p.upf.sim(simulate, &p.conf.SimInfo)

		if !simulate.keepGoing() {
			return
		}
	}

	p.mustInit()

	go func() {
		if err := p.httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalln("http server failed", err)
		}

		log.Infoln("http server closed")
	}()

	//http.HandleFunc("/registergw", RegisterGw)
	//server := http.Server{Addr: ":8082"}
	//log.Traceln("starting http server on 8082")
	//go server.ListenAndServe()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	signal.Notify(sig, syscall.SIGTERM)

	go func() {
		oscall := <-sig
		log.Infof("System call received: %+v", oscall)
		p.Stop()
	}()
	//fmt.Println("parham log : calling PushPFCPInfo")
	//lAddr := p.node.LocalAddr().String()
	//PushPFCPInfo(lAddr)
	fmt.Println("parham log : calling PushPFCPInfoNew")
	fmt.Println("upf info before calling PushPFCPInfoNew :")
	fmt.Println("dnn = ", p.node.upf.Dnn)
	fmt.Println("AccessIP = ", p.node.upf.AccessIP)
	fmt.Println("CoreIP = ", p.node.upf.CoreIP)
	fmt.Println("nodeID = ", p.node.upf.NodeID)

	//PushPFCPInfoNew(p.node.upf)
	p.node.RegisterTolb(enterlb)
	p.node.RegisterTolb(exitlb)
	// blocking
	p.node.Serve()
}

type PfcpInfo struct {
	Ip  string `json:"ip"`
	Upf *upf   `json:"upf"`
}

func PushPFCPInfo(lAddr string) error {
	time.Sleep(15 * time.Second)
	done := false
	var conn net.Conn
	var err error

	for !done {
		conn, err = reuse.Dial("tcp", lAddr, "upf:8806")
		if err != nil {
			log.Errorln("dial socket failed", err)
			time.Sleep(1 * time.Second)
		} else {
			done = true
		}
	}
	fmt.Println("parham log : send pfcp info from:", conn.LocalAddr(), "to:", conn.RemoteAddr())
	fmt.Println("parham log : local address = ", conn.LocalAddr().String())
	pfcpinfo := PfcpInfo{
		Ip: conn.LocalAddr().String(),
	}
	rawpfcpinfo, err := json.Marshal(pfcpinfo)
	if err != nil {
		return err
	}

	_, err = http.Post("upf:8081/v1/register/pcfp", "application/json", bytes.NewBuffer(rawpfcpinfo))
	if err != nil {
		return err
	}
	fmt.Println("parham log : pfcp added to pfcplb")

	return nil
}

func PushPFCPInfoNew(upf *upf) {
	fmt.Println("waiting to register both Gateways")
	for {
		if upf.accessGwRegistered && upf.coreGwRegistered {
			break
		}
		time.Sleep(1 * time.Second)
	}
	fmt.Println("registered both Gateways")
	fmt.Println("upf info inside PushPFCPInfoNew :")
	fmt.Println("dnn = ", upf.Dnn)
	fmt.Println("AccessIP = ", upf.AccessIP)
	fmt.Println("CoreIP = ", upf.CoreIP)
	fmt.Println("nodeID = ", upf.NodeID)
	// get IP
	ip_str := GetLocalIP()
	pfcpInfo := &PfcpInfo{
		Ip:  ip_str,
		Upf: upf,
	}
	fmt.Println("parham log : local ip = ", ip_str)
	pfcpInfoJson, _ := json.Marshal(pfcpInfo)

	fmt.Printf("parham log : json encoded pfcpInfo [%s] ", pfcpInfoJson)

	// change the IP here
	requestURL := "http://upf-http:8081/"
	jsonBody := []byte(pfcpInfoJson)

	bodyReader := bytes.NewReader(jsonBody)
	req, err := http.NewRequest(http.MethodPost, requestURL, bodyReader)
	if err != nil {
		log.Errorf("client: could not create request: %s\n", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := http.Client{
		Timeout: 10 * time.Second,
	}
	//upfhttpIP := net.ParseIP("upf")
	//upfIP := net.ParseIP("upf-http")
	//fmt.Println("parham log : ip of upfIP and upfhttpIP =", upfIP, upfhttpIP)
	done := false
	for !done {
		resp, err := client.Do(req)
		if err != nil {
			log.Errorf("client: error making http request: %s\n", err)
			time.Sleep(1 * time.Second)
		} else {
			done = true
			//body, err := io.ReadAll(resp.Body)
			if err != nil {
				log.Errorf("error reading http respose: %s\n", err)
			} else {
				fmt.Println("parham log : resp header = ", resp.Header)
				fmt.Println("parham log : resp status = ", resp.Status)
			}

			return
		}
	}
	// waiting for http response

}

// GetLocalIP returns ip of first non loopback interface in string
func GetLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, address := range addrs {
		// check the address type and if it is not a loopback the display it
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return ""
}

func GetMac(interfaceName string) string {

	// Get the list of network interfaces.
	ifaces, err := net.Interfaces()
	if err != nil {
		fmt.Println("Error:", err)
		return ""
	}

	// Find the interface with the specified name.
	var targetInterface net.Interface
	for _, iface := range ifaces {
		if iface.Name == interfaceName {
			targetInterface = iface
			break
		}
	}

	if targetInterface.Name == "" {
		return ""
	}

	return targetInterface.HardwareAddr.String()
}

// Stop sends cancellation signal to main Go routine and waits for shutdown to complete.
func (p *PFCPIface) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	ctxHttpShutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer func() {
		cancel()
	}()

	if err := p.httpSrv.Shutdown(ctxHttpShutdown); err != nil {
		log.Errorln("Failed to shutdown http: ", err)
	}

	p.node.Stop()

	// Wait for PFCP node shutdown
	p.node.Done()
}
