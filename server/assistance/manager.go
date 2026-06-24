package assistance

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net"
	"sync"
	"time"
)

type Status string

const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusPaused   Status = "paused"
	StatusClosed   Status = "closed"
)

type PortRequest struct {
	LocalPort  int    `json:"localPort"`
	ProxyName  string `json:"proxyName"`
	Type       string `json:"type"`
	RemotePort int    `json:"remotePort"`
	RemoteAddr string `json:"remoteAddr"`
}

type Application struct {
	Code       string        `json:"code"`
	RunID      string        `json:"-"`
	User       string        `json:"user"`
	ClientKey  string        `json:"clientKey"`
	Ports      []PortRequest `json:"ports"`
	Remark     string        `json:"remark"`
	Status     Status        `json:"status"`
	CreatedAt  time.Time     `json:"createdAt"`
	ApprovedAt *time.Time    `json:"approvedAt,omitempty"`
	ClosedAt   *time.Time    `json:"closedAt,omitempty"`
}

type Manager struct {
	mu           sync.RWMutex
	applications map[string]*Application // code -> application
	proxyIndex   map[string]string       // proxyName -> code
	publicAddr   string                  // public IP for remote address display
}

func NewManager(publicAddr string) *Manager {
	return &Manager{
		applications: make(map[string]*Application),
		proxyIndex:   make(map[string]string),
		publicAddr:   publicAddr,
	}
}

// generateCode generates a random 6-character uppercase alphanumeric code
func generateCode() string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	code := make([]byte, 6)
	for i := range code {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		code[i] = chars[n.Int64()]
	}
	return string(code)
}

func (m *Manager) CreateApplication(runID, user, clientKey string, ports []int, types []string, remark string) *Application {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Generate unique code
	var code string
	for {
		code = generateCode()
		if _, exists := m.applications[code]; !exists {
			break
		}
	}

	// Default to tcp if no types specified
	if len(types) == 0 {
		types = []string{"tcp"}
	}

	// Build port requests with proxy names, one for each port x type combination
	portRequests := make([]PortRequest, 0, len(ports)*len(types))
	idx := 0
	for _, port := range ports {
		for _, proxyType := range types {
			proxyName := fmt.Sprintf("assist-%s-%d", code, idx)
			portRequests = append(portRequests, PortRequest{
				LocalPort: port,
				ProxyName: proxyName,
				Type:      proxyType,
			})
			m.proxyIndex[proxyName] = code
			idx++
		}
	}

	app := &Application{
		Code:      code,
		RunID:     runID,
		User:      user,
		ClientKey: clientKey,
		Ports:     portRequests,
		Remark:    remark,
		Status:    StatusPending,
		CreatedAt: time.Now(),
	}
	m.applications[code] = app
	return app
}

func (m *Manager) GetByCode(code string) (*Application, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	app, ok := m.applications[code]
	return app, ok
}

func (m *Manager) ListAll() []*Application {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Application, 0, len(m.applications))
	for _, app := range m.applications {
		result = append(result, app)
	}
	return result
}

func (m *Manager) Approve(code string) (*Application, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	app, ok := m.applications[code]
	if !ok {
		return nil, fmt.Errorf("application not found: %s", code)
	}
	if app.Status != StatusPending {
		return nil, fmt.Errorf("application status is %s, not pending", app.Status)
	}

	now := time.Now()
	app.Status = StatusApproved
	app.ApprovedAt = &now
	return app, nil
}

func (m *Manager) OnProxyRegistered(proxyName, remoteAddr string, remotePort int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	code, ok := m.proxyIndex[proxyName]
	if !ok {
		return
	}

	app, ok := m.applications[code]
	if !ok {
		return
	}

	// Build display address: use publicAddr if configured, else use raw remoteAddr
	displayAddr := remoteAddr
	if m.publicAddr != "" && remotePort > 0 {
		displayAddr = net.JoinHostPort(m.publicAddr, fmt.Sprintf("%d", remotePort))
	}

	for i := range app.Ports {
		if app.Ports[i].ProxyName == proxyName {
			app.Ports[i].RemoteAddr = displayAddr
			app.Ports[i].RemotePort = remotePort
			break
		}
	}
}

// IsApprovedProxy returns true if the proxy name belongs to an approved assistance application.
func (m *Manager) IsApprovedProxy(proxyName string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	code, ok := m.proxyIndex[proxyName]
	if !ok {
		return false
	}
	app, ok := m.applications[code]
	if !ok {
		return false
	}
	return app.Status == StatusApproved || app.Status == StatusPaused
}

func (m *Manager) Close(code string) (*Application, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	app, ok := m.applications[code]
	if !ok {
		return nil, fmt.Errorf("application not found: %s", code)
	}
	if app.Status != StatusApproved && app.Status != StatusPaused {
		return nil, fmt.Errorf("application status is %s, cannot close", app.Status)
	}

	now := time.Now()
	app.Status = StatusClosed
	app.ClosedAt = &now
	return app, nil
}

func (m *Manager) Pause(code string) (*Application, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	app, ok := m.applications[code]
	if !ok {
		return nil, fmt.Errorf("application not found: %s", code)
	}
	if app.Status != StatusApproved {
		return nil, fmt.Errorf("application status is %s, cannot pause", app.Status)
	}

	app.Status = StatusPaused
	return app, nil
}

func (m *Manager) Resume(code string) (*Application, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	app, ok := m.applications[code]
	if !ok {
		return nil, fmt.Errorf("application not found: %s", code)
	}
	if app.Status != StatusPaused {
		return nil, fmt.Errorf("application status is %s, cannot resume", app.Status)
	}

	app.Status = StatusApproved
	return app, nil
}

// RejectPending rejects a pending application without approval
func (m *Manager) RejectPending(code string) (*Application, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	app, ok := m.applications[code]
	if !ok {
		return nil, fmt.Errorf("application not found: %s", code)
	}
	if app.Status != StatusPending {
		return nil, fmt.Errorf("application status is %s, not pending", app.Status)
	}

	now := time.Now()
	app.Status = StatusClosed
	app.ClosedAt = &now
	return app, nil
}

func (m *Manager) GetProxyNames(code string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	app, ok := m.applications[code]
	if !ok {
		return nil
	}

	names := make([]string, len(app.Ports))
	for i, p := range app.Ports {
		names[i] = p.ProxyName
	}
	return names
}

func (m *Manager) RemoveByRunID(runID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for code, app := range m.applications {
		if app.RunID == runID {
			// Clean up proxy index
			for _, p := range app.Ports {
				delete(m.proxyIndex, p.ProxyName)
			}
			delete(m.applications, code)
		}
	}
}
