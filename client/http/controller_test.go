package http

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"

	"github.com/fatedier/frp/client/configmgmt"
	"github.com/fatedier/frp/client/http/model"
	"github.com/fatedier/frp/client/proxy"
	v1 "github.com/fatedier/frp/pkg/config/v1"
	httppkg "github.com/fatedier/frp/pkg/util/http"
)

type fakeConfigManager struct {
	reloadFromFileFn      func(strict bool) error
	readConfigFileFn      func() (string, error)
	writeConfigFileFn     func(content []byte) error
	getProxyStatusFn      func() []*proxy.WorkingStatus
	isStoreProxyEnabledFn func(name string) bool
	storeEnabledFn        func() bool
	getProxyConfigFn      func(name string) (v1.ProxyConfigurer, bool)
	getVisitorConfigFn    func(name string) (v1.VisitorConfigurer, bool)

	listStoreProxiesFn  func() ([]v1.ProxyConfigurer, error)
	getStoreProxyFn     func(name string) (v1.ProxyConfigurer, error)
	createStoreProxyFn  func(cfg v1.ProxyConfigurer) (v1.ProxyConfigurer, error)
	updateStoreProxyFn  func(name string, cfg v1.ProxyConfigurer) (v1.ProxyConfigurer, error)
	deleteStoreProxyFn  func(name string) error
	listStoreVisitorsFn func() ([]v1.VisitorConfigurer, error)
	getStoreVisitorFn   func(name string) (v1.VisitorConfigurer, error)
	createStoreVisitFn  func(cfg v1.VisitorConfigurer) (v1.VisitorConfigurer, error)
	updateStoreVisitFn  func(name string, cfg v1.VisitorConfigurer) (v1.VisitorConfigurer, error)
	deleteStoreVisitFn  func(name string) error
	gracefulCloseFn     func(d time.Duration)
}

func (m *fakeConfigManager) ReloadFromFile(strict bool) error {
	if m.reloadFromFileFn != nil {
		return m.reloadFromFileFn(strict)
	}
	return nil
}

func (m *fakeConfigManager) ReadConfigFile() (string, error) {
	if m.readConfigFileFn != nil {
		return m.readConfigFileFn()
	}
	return "", nil
}

func (m *fakeConfigManager) WriteConfigFile(content []byte) error {
	if m.writeConfigFileFn != nil {
		return m.writeConfigFileFn(content)
	}
	return nil
}

func (m *fakeConfigManager) GetProxyStatus() []*proxy.WorkingStatus {
	if m.getProxyStatusFn != nil {
		return m.getProxyStatusFn()
	}
	return nil
}

func (m *fakeConfigManager) IsStoreProxyEnabled(name string) bool {
	if m.isStoreProxyEnabledFn != nil {
		return m.isStoreProxyEnabledFn(name)
	}
	return false
}

func (m *fakeConfigManager) StoreEnabled() bool {
	if m.storeEnabledFn != nil {
		return m.storeEnabledFn()
	}
	return false
}

func (m *fakeConfigManager) GetProxyConfig(name string) (v1.ProxyConfigurer, bool) {
	if m.getProxyConfigFn != nil {
		return m.getProxyConfigFn(name)
	}
	return nil, false
}

func (m *fakeConfigManager) GetVisitorConfig(name string) (v1.VisitorConfigurer, bool) {
	if m.getVisitorConfigFn != nil {
		return m.getVisitorConfigFn(name)
	}
	return nil, false
}

func (m *fakeConfigManager) ListStoreProxies() ([]v1.ProxyConfigurer, error) {
	if m.listStoreProxiesFn != nil {
		return m.listStoreProxiesFn()
	}
	return nil, nil
}

func (m *fakeConfigManager) GetStoreProxy(name string) (v1.ProxyConfigurer, error) {
	if m.getStoreProxyFn != nil {
		return m.getStoreProxyFn(name)
	}
	return nil, nil
}

func (m *fakeConfigManager) CreateStoreProxy(cfg v1.ProxyConfigurer) (v1.ProxyConfigurer, error) {
	if m.createStoreProxyFn != nil {
		return m.createStoreProxyFn(cfg)
	}
	return cfg, nil
}

func (m *fakeConfigManager) UpdateStoreProxy(name string, cfg v1.ProxyConfigurer) (v1.ProxyConfigurer, error) {
	if m.updateStoreProxyFn != nil {
		return m.updateStoreProxyFn(name, cfg)
	}
	return cfg, nil
}

func (m *fakeConfigManager) DeleteStoreProxy(name string) error {
	if m.deleteStoreProxyFn != nil {
		return m.deleteStoreProxyFn(name)
	}
	return nil
}

func (m *fakeConfigManager) ListStoreVisitors() ([]v1.VisitorConfigurer, error) {
	if m.listStoreVisitorsFn != nil {
		return m.listStoreVisitorsFn()
	}
	return nil, nil
}

func (m *fakeConfigManager) GetStoreVisitor(name string) (v1.VisitorConfigurer, error) {
	if m.getStoreVisitorFn != nil {
		return m.getStoreVisitorFn(name)
	}
	return nil, nil
}

func (m *fakeConfigManager) CreateStoreVisitor(cfg v1.VisitorConfigurer) (v1.VisitorConfigurer, error) {
	if m.createStoreVisitFn != nil {
		return m.createStoreVisitFn(cfg)
	}
	return cfg, nil
}

func (m *fakeConfigManager) UpdateStoreVisitor(name string, cfg v1.VisitorConfigurer) (v1.VisitorConfigurer, error) {
	if m.updateStoreVisitFn != nil {
		return m.updateStoreVisitFn(name, cfg)
	}
	return cfg, nil
}

func (m *fakeConfigManager) DeleteStoreVisitor(name string) error {
	if m.deleteStoreVisitFn != nil {
		return m.deleteStoreVisitFn(name)
	}
	return nil
}

func (m *fakeConfigManager) GracefulClose(d time.Duration) {
	if m.gracefulCloseFn != nil {
		m.gracefulCloseFn(d)
	}
}

func newRawTCPProxyConfig(name string) *v1.TCPProxyConfig {
	return &v1.TCPProxyConfig{
		ProxyBaseConfig: v1.ProxyBaseConfig{
			Name: name,
			Type: "tcp",
			ProxyBackend: v1.ProxyBackend{
				LocalPort: 10080,
			},
		},
	}
}

func TestBuildProxyStatusRespStoreSourceEnabled(t *testing.T) {
	status := &proxy.WorkingStatus{
		Name:       "shared-proxy",
		Type:       "tcp",
		Phase:      proxy.ProxyPhaseRunning,
		RemoteAddr: ":8080",
		Cfg:        newRawTCPProxyConfig("shared-proxy"),
	}

	controller := &Controller{
		serverAddr: "127.0.0.1",
		manager: &fakeConfigManager{
			isStoreProxyEnabledFn: func(name string) bool {
				return name == "shared-proxy"
			},
		},
	}

	resp := controller.buildProxyStatusResp(status)
	if resp.Source != "store" {
		t.Fatalf("unexpected source: %q", resp.Source)
	}
	if resp.RemoteAddr != "127.0.0.1:8080" {
		t.Fatalf("unexpected remote addr: %q", resp.RemoteAddr)
	}
}

func TestReloadErrorMapping(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		expectedCode int
	}{
		{name: "invalid arg", err: fmtError(configmgmt.ErrInvalidArgument, "bad cfg"), expectedCode: http.StatusBadRequest},
		{name: "apply fail", err: fmtError(configmgmt.ErrApplyConfig, "reload failed"), expectedCode: http.StatusInternalServerError},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			controller := &Controller{
				manager: &fakeConfigManager{reloadFromFileFn: func(bool) error { return tc.err }},
			}
			ctx := httppkg.NewContext(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/reload", nil))
			_, err := controller.Reload(ctx)
			if err == nil {
				t.Fatal("expected error")
			}
			assertHTTPCode(t, err, tc.expectedCode)
		})
	}
}

func fmtError(sentinel error, msg string) error {
	return fmt.Errorf("%w: %s", sentinel, msg)
}

func assertHTTPCode(t *testing.T, err error, expected int) {
	t.Helper()
	var httpErr *httppkg.Error
	if !errors.As(err, &httpErr) {
		t.Fatalf("unexpected error type: %T", err)
	}
	if httpErr.Code != expected {
		t.Fatalf("unexpected status code: got %d, want %d", httpErr.Code, expected)
	}
}

func TestGetProxyConfigFromManager(t *testing.T) {
	controller := &Controller{
		manager: &fakeConfigManager{
			getProxyConfigFn: func(name string) (v1.ProxyConfigurer, bool) {
				if name == "ssh" {
					cfg := &v1.TCPProxyConfig{
						ProxyBaseConfig: v1.ProxyBaseConfig{
							Name: "ssh",
							Type: "tcp",
							ProxyBackend: v1.ProxyBackend{
								LocalPort: 22,
							},
						},
					}
					return cfg, true
				}
				return nil, false
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/proxy/ssh/config", nil)
	req = mux.SetURLVars(req, map[string]string{"name": "ssh"})
	ctx := httppkg.NewContext(httptest.NewRecorder(), req)

	resp, err := controller.GetProxyConfig(ctx)
	if err != nil {
		t.Fatalf("get proxy config: %v", err)
	}
	payload, ok := resp.(model.ProxyDefinition)
	if !ok {
		t.Fatalf("unexpected response type: %T", resp)
	}
	if payload.Name != "ssh" || payload.Type != "tcp" || payload.TCP == nil {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestGetProxyConfigNotFound(t *testing.T) {
	controller := &Controller{
		manager: &fakeConfigManager{
			getProxyConfigFn: func(name string) (v1.ProxyConfigurer, bool) {
				return nil, false
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/proxy/missing/config", nil)
	req = mux.SetURLVars(req, map[string]string{"name": "missing"})
	ctx := httppkg.NewContext(httptest.NewRecorder(), req)

	_, err := controller.GetProxyConfig(ctx)
	if err == nil {
		t.Fatal("expected error")
	}
	assertHTTPCode(t, err, http.StatusNotFound)
}

func TestGetVisitorConfigFromManager(t *testing.T) {
	controller := &Controller{
		manager: &fakeConfigManager{
			getVisitorConfigFn: func(name string) (v1.VisitorConfigurer, bool) {
				if name == "my-stcp" {
					cfg := &v1.STCPVisitorConfig{
						VisitorBaseConfig: v1.VisitorBaseConfig{
							Name:       "my-stcp",
							Type:       "stcp",
							ServerName: "server1",
							BindPort:   9000,
						},
					}
					return cfg, true
				}
				return nil, false
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/visitor/my-stcp/config", nil)
	req = mux.SetURLVars(req, map[string]string{"name": "my-stcp"})
	ctx := httppkg.NewContext(httptest.NewRecorder(), req)

	resp, err := controller.GetVisitorConfig(ctx)
	if err != nil {
		t.Fatalf("get visitor config: %v", err)
	}
	payload, ok := resp.(model.VisitorDefinition)
	if !ok {
		t.Fatalf("unexpected response type: %T", resp)
	}
	if payload.Name != "my-stcp" || payload.Type != "stcp" || payload.STCP == nil {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestGetVisitorConfigNotFound(t *testing.T) {
	controller := &Controller{
		manager: &fakeConfigManager{
			getVisitorConfigFn: func(name string) (v1.VisitorConfigurer, bool) {
				return nil, false
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/visitor/missing/config", nil)
	req = mux.SetURLVars(req, map[string]string{"name": "missing"})
	ctx := httppkg.NewContext(httptest.NewRecorder(), req)

	_, err := controller.GetVisitorConfig(ctx)
	if err == nil {
		t.Fatal("expected error")
	}
	assertHTTPCode(t, err, http.StatusNotFound)
}
