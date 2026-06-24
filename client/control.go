// Copyright 2017 fatedier, fatedier@gmail.com
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

package client

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fatedier/frp/client/proxy"
	"github.com/fatedier/frp/client/visitor"
	"github.com/fatedier/frp/pkg/auth"
	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/fatedier/frp/pkg/msg"
	"github.com/fatedier/frp/pkg/naming"
	"github.com/fatedier/frp/pkg/transport"
	"github.com/fatedier/frp/pkg/util/wait"
	"github.com/fatedier/frp/pkg/util/xlog"
	"github.com/fatedier/frp/pkg/vnet"
)

type SessionContext struct {
	// The client common configuration.
	Common *v1.ClientCommonConfig

	// Unique ID obtained from frps.
	// It should be attached to the login message when reconnecting.
	RunID string
	// Underlying control connection. Once conn is closed, the msgDispatcher and the entire Control will exit.
	Conn *msg.Conn
	// Auth runtime used for login, heartbeats, and encryption.
	Auth *auth.ClientAuth
	// Connector is used to create message connections to frps.
	Connector MessageConnector
	// Virtual net controller
	VnetController *vnet.Controller
}

type Control struct {
	// service context
	ctx context.Context
	xl  *xlog.Logger

	// session context
	sessionCtx *SessionContext

	// manage all proxies
	pm *proxy.Manager

	// manage all visitors
	vm *visitor.Manager

	doneCh chan struct{}

	// of time.Time, last time got the Pong message
	lastPong atomic.Value

	// The role of msgTransporter is similar to HTTP2.
	// It allows multiple messages to be sent simultaneously on the same control connection.
	// The server's response messages will be dispatched to the corresponding waiting goroutines based on the laneKey and message type.
	msgTransporter transport.MessageTransporter

	// msgDispatcher is a wrapper for control connection.
	// It provides a channel for sending messages, and you can register handlers to process messages based on their respective types.
	msgDispatcher *msg.Dispatcher

	assistanceProxies map[string][]v1.ProxyConfigurer
	assistanceMu      sync.Mutex

	applications   map[string]*applicationInfo
	applicationsMu sync.RWMutex
}

type applicationInfo struct {
	Code      string
	Status    string
	Ports     []applicationPort
	CreatedAt time.Time
}

type applicationPort struct {
	LocalPort  int
	Type       string
	RemotePort int
}

func generateTransactionID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func NewControl(ctx context.Context, sessionCtx *SessionContext) (*Control, error) {
	// new xlog instance
	ctl := &Control{
		ctx:        ctx,
		xl:         xlog.FromContextSafe(ctx),
		sessionCtx: sessionCtx,
		doneCh:     make(chan struct{}),
	}
	ctl.lastPong.Store(time.Now())

	ctl.msgDispatcher = msg.NewDispatcher(sessionCtx.Conn)
	ctl.registerMsgHandlers()
	ctl.msgTransporter = transport.NewMessageTransporter(ctl.msgDispatcher)

	ctl.pm = proxy.NewManager(ctl.ctx, sessionCtx.Common, sessionCtx.Auth.EncryptionKey(), ctl.msgTransporter, sessionCtx.VnetController)
	ctl.vm = visitor.NewManager(ctl.ctx, sessionCtx.RunID, sessionCtx.Common,
		ctl.connectServer, ctl.msgTransporter, sessionCtx.VnetController)
	return ctl, nil
}

func (ctl *Control) Run(proxyCfgs []v1.ProxyConfigurer, visitorCfgs []v1.VisitorConfigurer) {
	go ctl.worker()

	// start all proxies
	ctl.pm.UpdateAll(proxyCfgs)

	// start all visitors
	ctl.vm.UpdateAll(visitorCfgs)
}

func (ctl *Control) SetInWorkConnCallback(cb func(*v1.ProxyBaseConfig, net.Conn, *msg.StartWorkConn) bool) {
	ctl.pm.SetInWorkConnCallback(cb)
}

func (ctl *Control) handleReqWorkConn(_ msg.Message) {
	xl := ctl.xl
	workConn, err := ctl.connectServer()
	if err != nil {
		xl.Warnf("start new connection to server error: %v", err)
		return
	}

	m := &msg.NewWorkConn{
		RunID: ctl.sessionCtx.RunID,
	}
	if err = ctl.sessionCtx.Auth.Setter.SetNewWorkConn(m); err != nil {
		xl.Warnf("error during NewWorkConn authentication: %v", err)
		workConn.Close()
		return
	}
	if err = workConn.WriteMsg(m); err != nil {
		xl.Warnf("work connection write to server error: %v", err)
		workConn.Close()
		return
	}

	var startMsg msg.StartWorkConn
	if err = workConn.ReadMsgInto(&startMsg); err != nil {
		xl.Tracef("work connection closed before response StartWorkConn message: %v", err)
		workConn.Close()
		return
	}
	if startMsg.Error != "" {
		xl.Errorf("StartWorkConn contains error: %s", startMsg.Error)
		workConn.Close()
		return
	}

	startMsg.ProxyName = naming.StripUserPrefix(ctl.sessionCtx.Common.User, startMsg.ProxyName)

	// dispatch this work connection to related proxy
	ctl.pm.HandleWorkConn(startMsg.ProxyName, workConn, &startMsg)
}

func (ctl *Control) handleNewProxyResp(m msg.Message) {
	xl := ctl.xl
	inMsg := m.(*msg.NewProxyResp)
	// Server will return NewProxyResp message to each NewProxy message.
	// Start a new proxy handler if no error got
	proxyName := naming.StripUserPrefix(ctl.sessionCtx.Common.User, inMsg.ProxyName)
	err := ctl.pm.StartProxy(proxyName, inMsg.RemoteAddr, inMsg.Error)
	if err != nil {
		xl.Warnf("[%s] start error: %v", proxyName, err)
	} else {
		xl.Infof("[%s] start proxy success", proxyName)
	}
}

func (ctl *Control) handleNatHoleResp(m msg.Message) {
	xl := ctl.xl
	inMsg := m.(*msg.NatHoleResp)

	// Dispatch the NatHoleResp message to the related proxy.
	ok := ctl.msgTransporter.DispatchWithType(inMsg, msg.TypeNameNatHoleResp, inMsg.TransactionID)
	if !ok {
		xl.Tracef("dispatch NatHoleResp message to related proxy error")
	}
}

func (ctl *Control) handlePong(m msg.Message) {
	xl := ctl.xl
	inMsg := m.(*msg.Pong)

	if inMsg.Error != "" {
		xl.Errorf("pong message contains error: %s", inMsg.Error)
		ctl.closeSession()
		return
	}
	ctl.lastPong.Store(time.Now())
	xl.Debugf("receive heartbeat from server")
}

// closeSession closes the control connection.
func (ctl *Control) closeSession() {
	ctl.sessionCtx.Conn.Close()
	ctl.sessionCtx.Connector.Close()
}

func (ctl *Control) Close() error {
	return ctl.GracefulClose(0)
}

func (ctl *Control) GracefulClose(d time.Duration) error {
	ctl.pm.Close()
	ctl.vm.Close()

	time.Sleep(d)

	ctl.closeSession()
	return nil
}

// Done returns a channel that will be closed after all resources are released
func (ctl *Control) Done() <-chan struct{} {
	return ctl.doneCh
}

// connectServer return a new connection to frps
func (ctl *Control) connectServer() (*msg.Conn, error) {
	return ctl.sessionCtx.Connector.Connect()
}

func (ctl *Control) registerMsgHandlers() {
	ctl.msgDispatcher.RegisterHandler(&msg.ReqWorkConn{}, msg.AsyncHandler(ctl.handleReqWorkConn))
	ctl.msgDispatcher.RegisterHandler(&msg.NewProxyResp{}, ctl.handleNewProxyResp)
	ctl.msgDispatcher.RegisterHandler(&msg.NatHoleResp{}, ctl.handleNatHoleResp)
	ctl.msgDispatcher.RegisterHandler(&msg.Pong{}, ctl.handlePong)
	ctl.msgDispatcher.RegisterHandler(&msg.PortApplicationResp{}, ctl.handlePortApplicationResp)
	ctl.msgDispatcher.RegisterHandler(&msg.ApprovalNotify{}, ctl.handleApprovalNotify)
	ctl.msgDispatcher.RegisterHandler(&msg.CloseAssistance{}, ctl.handleCloseAssistance)
	ctl.msgDispatcher.RegisterHandler(&msg.PauseAssistance{}, ctl.handlePauseAssistance)
	ctl.msgDispatcher.RegisterHandler(&msg.ResumeAssistance{}, ctl.handleResumeAssistance)
	ctl.msgDispatcher.RegisterHandler(&msg.DisconnectAssistance{}, ctl.handleDisconnectAssistance)
}

func (ctl *Control) handlePortApplicationResp(m msg.Message) {
	xl := ctl.xl
	inMsg := m.(*msg.PortApplicationResp)
	if inMsg.TransactionID != "" {
		ctl.msgTransporter.DispatchWithType(m, "PortApplicationResp", inMsg.TransactionID)
		return
	}
	if inMsg.Error != "" {
		xl.Errorf("port application failed: %s", inMsg.Error)
		return
	}
	xl.Infof("port application submitted, code: %s, status: %s", inMsg.Code, inMsg.Status)
}

func (ctl *Control) handleApprovalNotify(m msg.Message) {
	xl := ctl.xl
	inMsg := m.(*msg.ApprovalNotify)
	xl.Infof("assistance approved, code: %s, creating %d proxies", inMsg.Code, len(inMsg.Ports))

	var cfgs []v1.ProxyConfigurer
	for _, p := range inMsg.Ports {
		proxyType := p.Type
		if proxyType == "" {
			proxyType = "tcp"
		}

		switch proxyType {
		case "udp":
			cfg := &v1.UDPProxyConfig{}
			cfg.Name = p.ProxyName
			cfg.Type = "udp"
			cfg.LocalIP = "127.0.0.1"
			cfg.LocalPort = p.LocalPort
			cfg.RemotePort = p.RemotePort
			cfgs = append(cfgs, cfg)
		default: // tcp
			cfg := &v1.TCPProxyConfig{}
			cfg.Name = p.ProxyName
			cfg.Type = "tcp"
			cfg.LocalIP = "127.0.0.1"
			cfg.LocalPort = p.LocalPort
			cfg.RemotePort = p.RemotePort
			cfgs = append(cfgs, cfg)
		}
	}

	ctl.assistanceMu.Lock()
	if ctl.assistanceProxies == nil {
		ctl.assistanceProxies = make(map[string][]v1.ProxyConfigurer)
	}
	ctl.assistanceProxies[inMsg.Code] = cfgs
	ctl.assistanceMu.Unlock()

	ctl.approveApplication(inMsg.Code, inMsg.Ports)
	ctl.updateAssistanceProxies()
}

func (ctl *Control) handleCloseAssistance(m msg.Message) {
	xl := ctl.xl
	inMsg := m.(*msg.CloseAssistance)
	xl.Infof("assistance closed, code: %s, reason: %s", inMsg.Code, inMsg.Reason)

	ctl.assistanceMu.Lock()
	delete(ctl.assistanceProxies, inMsg.Code)
	ctl.assistanceMu.Unlock()

	ctl.closeApplication(inMsg.Code)
	ctl.updateAssistanceProxies()
}

func (ctl *Control) handlePauseAssistance(m msg.Message) {
	xl := ctl.xl
	inMsg := m.(*msg.PauseAssistance)
	xl.Infof("assistance paused, code: %s, reason: %s", inMsg.Code, inMsg.Reason)

	ctl.assistanceMu.Lock()
	delete(ctl.assistanceProxies, inMsg.Code)
	ctl.assistanceMu.Unlock()

	ctl.pauseApplication(inMsg.Code)
	ctl.updateAssistanceProxies()
}

func (ctl *Control) handleResumeAssistance(m msg.Message) {
	xl := ctl.xl
	inMsg := m.(*msg.ResumeAssistance)
	xl.Infof("assistance resumed, code: %s, creating %d proxies", inMsg.Code, len(inMsg.Ports))

	var cfgs []v1.ProxyConfigurer
	for _, p := range inMsg.Ports {
		proxyType := p.Type
		if proxyType == "" {
			proxyType = "tcp"
		}

		switch proxyType {
		case "udp":
			cfg := &v1.UDPProxyConfig{}
			cfg.Name = p.ProxyName
			cfg.Type = "udp"
			cfg.LocalIP = "127.0.0.1"
			cfg.LocalPort = p.LocalPort
			cfg.RemotePort = p.RemotePort
			cfgs = append(cfgs, cfg)
		default: // tcp
			cfg := &v1.TCPProxyConfig{}
			cfg.Name = p.ProxyName
			cfg.Type = "tcp"
			cfg.LocalIP = "127.0.0.1"
			cfg.LocalPort = p.LocalPort
			cfg.RemotePort = p.RemotePort
			cfgs = append(cfgs, cfg)
		}
	}

	ctl.assistanceMu.Lock()
	if ctl.assistanceProxies == nil {
		ctl.assistanceProxies = make(map[string][]v1.ProxyConfigurer)
	}
	ctl.assistanceProxies[inMsg.Code] = cfgs
	ctl.assistanceMu.Unlock()

	ctl.resumeApplication(inMsg.Code, inMsg.Ports)
	ctl.updateAssistanceProxies()
}

func (ctl *Control) handleDisconnectAssistance(m msg.Message) {
	xl := ctl.xl
	inMsg := m.(*msg.DisconnectAssistance)
	xl.Infof("assistance disconnect, code: %s, reason: %s - exiting process", inMsg.Code, inMsg.Reason)

	ctl.assistanceMu.Lock()
	delete(ctl.assistanceProxies, inMsg.Code)
	ctl.assistanceMu.Unlock()

	ctl.closeApplication(inMsg.Code)
	ctl.updateAssistanceProxies()

	// Give some time for the proxy list update to be sent, then exit
	time.Sleep(500 * time.Millisecond)
	os.Exit(0)
}

func (ctl *Control) updateAssistanceProxies() {
	ctl.assistanceMu.Lock()
	var allCfgs []v1.ProxyConfigurer
	for _, cfgs := range ctl.assistanceProxies {
		allCfgs = append(allCfgs, cfgs...)
	}
	ctl.assistanceMu.Unlock()

	ctl.pm.UpdateAll(allCfgs)
}

func (ctl *Control) addApplication(code, status string, ports []int, types []string) {
	ctl.applicationsMu.Lock()
	defer ctl.applicationsMu.Unlock()
	if ctl.applications == nil {
		ctl.applications = make(map[string]*applicationInfo)
	}

	// Default to tcp if no types specified
	if len(types) == 0 {
		types = []string{"tcp"}
	}

	// Build port mappings, one for each port x type combination
	portMappings := make([]applicationPort, 0, len(ports)*len(types))
	for _, port := range ports {
		for _, proxyType := range types {
			portMappings = append(portMappings, applicationPort{
				LocalPort: port,
				Type:      proxyType,
			})
		}
	}
	ctl.applications[code] = &applicationInfo{
		Code:      code,
		Status:    status,
		Ports:     portMappings,
		CreatedAt: time.Now(),
	}
}

func (ctl *Control) approveApplication(code string, ports []msg.ApprovedPort) {
	ctl.applicationsMu.Lock()
	defer ctl.applicationsMu.Unlock()
	app, ok := ctl.applications[code]
	if !ok {
		return
	}
	app.Status = "approved"
	remotePorts := make(map[string]int, len(ports))
	for _, p := range ports {
		remotePorts[p.ProxyName] = p.RemotePort
	}
	for i := range app.Ports {
		app.Ports[i].RemotePort = remotePorts[fmt.Sprintf("assist-%s-%d", code, i)]
	}
}

func (ctl *Control) closeApplication(code string) {
	ctl.applicationsMu.Lock()
	defer ctl.applicationsMu.Unlock()
	if app, ok := ctl.applications[code]; ok {
		app.Status = "closed"
	}
}

func (ctl *Control) pauseApplication(code string) {
	ctl.applicationsMu.Lock()
	defer ctl.applicationsMu.Unlock()
	if app, ok := ctl.applications[code]; ok {
		app.Status = "paused"
	}
}

func (ctl *Control) resumeApplication(code string, ports []msg.ApprovedPort) {
	ctl.applicationsMu.Lock()
	defer ctl.applicationsMu.Unlock()
	app, ok := ctl.applications[code]
	if !ok {
		return
	}
	app.Status = "approved"
	remotePorts := make(map[string]int, len(ports))
	for _, p := range ports {
		remotePorts[p.ProxyName] = p.RemotePort
	}
	for i := range app.Ports {
		app.Ports[i].RemotePort = remotePorts[fmt.Sprintf("assist-%s-%d", code, i)]
	}
}

// GetApplications returns all known assistance applications.
func (ctl *Control) GetApplications() []*applicationInfo {
	ctl.applicationsMu.RLock()
	defer ctl.applicationsMu.RUnlock()
	result := make([]*applicationInfo, 0, len(ctl.applications))
	for _, app := range ctl.applications {
		result = append(result, app)
	}
	return result
}

func (ctl *Control) SubmitApplication(ports []int, types []string, remark string) (string, error) {
	laneKey := generateTransactionID()
	appMsg := &msg.PortApplication{
		TransactionID: laneKey,
		Ports:         ports,
		Types:         types,
		Remark:        remark,
	}

	ctx, cancel := context.WithTimeout(ctl.ctx, 10*time.Second)
	defer cancel()
	respMsg, err := ctl.msgTransporter.Do(ctx, appMsg, laneKey, "PortApplicationResp")
	if err != nil {
		return "", fmt.Errorf("submit application error: %w", err)
	}

	resp := respMsg.(*msg.PortApplicationResp)
	if resp.Error != "" {
		return "", fmt.Errorf("submit application failed: %s", resp.Error)
	}

	ctl.addApplication(resp.Code, resp.Status, ports, types)
	ctl.xl.Infof("port application submitted, code: %s, status: %s", resp.Code, resp.Status)
	return resp.Code, nil
}

// heartbeatWorker sends heartbeat to server and check heartbeat timeout.
func (ctl *Control) heartbeatWorker() {
	xl := ctl.xl

	if ctl.sessionCtx.Common.Transport.HeartbeatInterval > 0 {
		// Send heartbeat to server.
		sendHeartBeat := func() (bool, error) {
			xl.Debugf("send heartbeat to server")
			pingMsg := &msg.Ping{}
			if err := ctl.sessionCtx.Auth.Setter.SetPing(pingMsg); err != nil {
				xl.Warnf("error during ping authentication: %v, skip sending ping message", err)
				return false, err
			}
			_ = ctl.msgDispatcher.Send(pingMsg)
			return false, nil
		}

		go wait.BackoffUntil(sendHeartBeat,
			wait.NewFastBackoffManager(wait.FastBackoffOptions{
				Duration:           time.Duration(ctl.sessionCtx.Common.Transport.HeartbeatInterval) * time.Second,
				InitDurationIfFail: time.Second,
				Factor:             2.0,
				Jitter:             0.1,
				MaxDuration:        time.Duration(ctl.sessionCtx.Common.Transport.HeartbeatInterval) * time.Second,
			}),
			true, ctl.doneCh,
		)
	}

	// Check heartbeat timeout.
	if ctl.sessionCtx.Common.Transport.HeartbeatInterval > 0 && ctl.sessionCtx.Common.Transport.HeartbeatTimeout > 0 {
		go wait.Until(func() {
			if time.Since(ctl.lastPong.Load().(time.Time)) > time.Duration(ctl.sessionCtx.Common.Transport.HeartbeatTimeout)*time.Second {
				xl.Warnf("heartbeat timeout")
				ctl.closeSession()
				return
			}
		}, time.Second, ctl.doneCh)
	}
}

func (ctl *Control) worker() {
	xl := ctl.xl
	go ctl.heartbeatWorker()
	go ctl.msgDispatcher.Run()

	<-ctl.msgDispatcher.Done()
	xl.Debugf("control message dispatcher exited")
	ctl.closeSession()

	ctl.pm.Close()
	ctl.vm.Close()
	close(ctl.doneCh)
}

func (ctl *Control) UpdateAllConfigurer(proxyCfgs []v1.ProxyConfigurer, visitorCfgs []v1.VisitorConfigurer) error {
	ctl.vm.UpdateAll(visitorCfgs)
	ctl.pm.UpdateAll(proxyCfgs)
	return nil
}
