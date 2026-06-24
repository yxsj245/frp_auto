package ports

import (
	"errors"
	"net"
	"sort"
	"strconv"
	"sync"
	"time"

	"k8s.io/utils/clock"

	"github.com/fatedier/frp/pkg/config/types"
)

const (
	MinPort                    = 1
	MaxPort                    = 65535
	MaxPortReservedDuration    = time.Duration(24) * time.Hour
	CleanReservedPortsInterval = time.Hour
)

var (
	ErrPortAlreadyUsed = errors.New("port already used")
	ErrPortNotAllowed  = errors.New("port not allowed")
	ErrPortUnAvailable = errors.New("port unavailable")
	ErrNoAvailablePort = errors.New("no available port")
)

type PortCtx struct {
	ProxyName  string
	Port       int
	Closed     bool
	UpdateTime time.Time
}

type Manager struct {
	reservedPorts map[string]*PortCtx
	usedPorts     map[int]*PortCtx
	freePorts     map[int]struct{}
	allowRanges   []types.PortsRange
	nextScanPort  int

	bindAddr string
	netType  string
	clock    clock.WithTicker
	mu       sync.Mutex
}

func NewManager(netType string, bindAddr string, allowPorts []types.PortsRange) *Manager {
	return newManagerWithClock(netType, bindAddr, allowPorts, clock.RealClock{})
}

func newManagerWithClock(netType string, bindAddr string, allowPorts []types.PortsRange, clk clock.WithTicker) *Manager {
	if clk == nil {
		clk = clock.RealClock{}
	}
	pm := &Manager{
		reservedPorts: make(map[string]*PortCtx),
		usedPorts:     make(map[int]*PortCtx),
		freePorts:     make(map[int]struct{}),
		allowRanges:   allowPorts,
		bindAddr:      bindAddr,
		netType:       netType,
		clock:         clk,
	}
	if len(allowPorts) > 0 {
		for _, pair := range allowPorts {
			if pair.Single > 0 {
				pm.freePorts[pair.Single] = struct{}{}
			} else {
				for i := pair.Start; i <= pair.End; i++ {
					pm.freePorts[i] = struct{}{}
				}
			}
		}
		// Set nextScanPort to the first port of the first range
		pm.nextScanPort = pm.getFirstRangePort()
	} else {
		for i := MinPort; i <= MaxPort; i++ {
			pm.freePorts[i] = struct{}{}
		}
		pm.nextScanPort = MinPort
	}
	go pm.cleanReservedPortsWorker()
	return pm
}

func (pm *Manager) Acquire(name string, port int) (realPort int, err error) {
	portCtx := &PortCtx{
		ProxyName:  name,
		Closed:     false,
		UpdateTime: pm.clock.Now(),
	}

	var ok bool

	pm.mu.Lock()
	defer func() {
		if err == nil {
			portCtx.Port = realPort
		}
		pm.mu.Unlock()
	}()

	// check reserved ports first
	if port == 0 {
		if ctx, ok := pm.reservedPorts[name]; ok {
			if pm.isPortAvailable(ctx.Port) {
				realPort = ctx.Port
				pm.markPortAcquiredLocked(name, realPort, portCtx)
				return
			}
		}
	}

	if port == 0 {
		// Sequential allocation: scan forward from the last allocated port
		realPort = pm.scanNextFreePort()
		if realPort == 0 {
			err = ErrNoAvailablePort
		} else {
			pm.markPortAcquiredLocked(name, realPort, portCtx)
		}
	} else {
		// specified port
		if _, ok = pm.freePorts[port]; ok {
			if pm.isPortAvailable(port) {
				realPort = port
				pm.markPortAcquiredLocked(name, realPort, portCtx)
			} else {
				err = ErrPortUnAvailable
			}
		} else {
			if _, ok = pm.usedPorts[port]; ok {
				err = ErrPortAlreadyUsed
			} else {
				err = ErrPortNotAllowed
			}
		}
	}
	return
}

// markPortAcquiredLocked records a successful acquisition. pm.mu must be held.
func (pm *Manager) markPortAcquiredLocked(name string, port int, portCtx *PortCtx) {
	pm.usedPorts[port] = portCtx
	pm.reservedPorts[name] = portCtx
	delete(pm.freePorts, port)
}

// getFirstRangePort returns the first port in the allowed ranges.
func (pm *Manager) getFirstRangePort() int {
	if len(pm.allowRanges) == 0 {
		return MinPort
	}
	first := pm.allowRanges[0]
	if first.Single > 0 {
		return first.Single
	}
	return first.Start
}

// scanNextFreePort scans forward from pm.nextScanPort to find the next free port.
// It wraps around the allowed ranges if needed.
func (pm *Manager) scanNextFreePort() int {
	if len(pm.allowRanges) == 0 {
		// No allowPorts configured, scan full range
		for retries := 0; retries < (MaxPort - MinPort + 1); retries++ {
			port := pm.nextScanPort
			pm.nextScanPort++
			if pm.nextScanPort > MaxPort {
				pm.nextScanPort = MinPort
			}
			if _, free := pm.freePorts[port]; free {
				if pm.isPortAvailable(port) {
					return port
				}
			}
		}
		return 0
	}

	// Collect and sort all allowed ports for sequential scanning
	allAllowed := make([]int, 0)
	for _, r := range pm.allowRanges {
		if r.Single > 0 {
			allAllowed = append(allAllowed, r.Single)
		} else {
			for i := r.Start; i <= r.End; i++ {
				allAllowed = append(allAllowed, i)
			}
		}
	}
	sort.Ints(allAllowed)

	if len(allAllowed) == 0 {
		return 0
	}

	// Find current position in the sorted list
	startIdx := 0
	for i, p := range allAllowed {
		if p >= pm.nextScanPort {
			startIdx = i
			break
		}
	}

	// Scan forward from current position
	for i := 0; i < len(allAllowed); i++ {
		idx := (startIdx + i) % len(allAllowed)
		port := allAllowed[idx]
		if _, free := pm.freePorts[port]; free {
			if pm.isPortAvailable(port) {
				// Advance cursor past this port for next allocation
				pm.nextScanPort = port + 1
				if pm.nextScanPort > allAllowed[len(allAllowed)-1] {
					pm.nextScanPort = allAllowed[0]
				}
				return port
			}
		}
	}
	return 0
}

func (pm *Manager) isPortAvailable(port int) bool {
	if pm.netType == "udp" {
		addr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(pm.bindAddr, strconv.Itoa(port)))
		if err != nil {
			return false
		}
		l, err := net.ListenUDP("udp", addr)
		if err != nil {
			return false
		}
		l.Close()
		return true
	}

	l, err := net.Listen(pm.netType, net.JoinHostPort(pm.bindAddr, strconv.Itoa(port)))
	if err != nil {
		return false
	}
	l.Close()
	return true
}

func (pm *Manager) Release(port int) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if ctx, ok := pm.usedPorts[port]; ok {
		pm.freePorts[port] = struct{}{}
		delete(pm.usedPorts, port)
		ctx.Closed = true
		ctx.UpdateTime = pm.clock.Now()
	}
}

// Release reserved port if it isn't used in last 24 hours.
func (pm *Manager) cleanReservedPortsWorker() {
	pm.cleanReservedPortsWorkerUntil(nil)
}

func (pm *Manager) cleanReservedPortsWorkerUntil(stopCh <-chan struct{}) {
	ticker := pm.clock.NewTicker(CleanReservedPortsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C():
			pm.cleanReservedPortsOnce()
		case <-stopCh:
			return
		}
	}
}

func (pm *Manager) cleanReservedPortsOnce() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for name, ctx := range pm.reservedPorts {
		if ctx.Closed && pm.clock.Since(ctx.UpdateTime) > MaxPortReservedDuration {
			delete(pm.reservedPorts, name)
		}
	}
}
