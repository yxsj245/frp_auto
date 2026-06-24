package sub

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/fatedier/frp/client"
	"github.com/fatedier/frp/pkg/config"
	"github.com/fatedier/frp/pkg/config/source"
	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/fatedier/frp/pkg/util/log"
)

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply for port forwarding (remote assistance)",
	RunE:  runApply,
}

var (
	applyPorts      []int
	applyTypes      []string
	applyRemark     string
	applyServer     string
	applyServerPort int
	applyToken      string
	applyWebPort    int
	applyWebAddr    string
	applyWebUser    string
	applyWebPwd     string
)

func init() {
	applyCmd.Flags().IntSliceVarP(&applyPorts, "ports", "p", nil, "local ports to expose (e.g. 3389,22)")
	applyCmd.Flags().StringSliceVarP(&applyTypes, "type", "t", []string{"tcp"}, "proxy types: tcp, udp or both (e.g. tcp,udp)")
	applyCmd.Flags().StringVarP(&applyRemark, "remark", "r", "", "remark for this application")
	applyCmd.Flags().StringVar(&applyServer, "server", "", "server address (overrides config)")
	applyCmd.Flags().IntVar(&applyServerPort, "server-port", 0, "server port (overrides config)")
	applyCmd.Flags().StringVar(&applyToken, "token", "", "auth token (overrides config)")
	applyCmd.Flags().IntVar(&applyWebPort, "web-port", 0, "web admin port (enable web dashboard)")
	applyCmd.Flags().StringVar(&applyWebAddr, "web-addr", "127.0.0.1", "web admin bind address")
	applyCmd.Flags().StringVar(&applyWebUser, "web-user", "", "web admin basic auth username")
	applyCmd.Flags().StringVar(&applyWebPwd, "web-pwd", "", "web admin basic auth password")
	rootCmd.AddCommand(applyCmd)
}

func runApply(cmd *cobra.Command, args []string) error {
	if len(applyPorts) == 0 {
		return fmt.Errorf("at least one port is required, use --ports/-p")
	}

	// 加载 common 配置（优先从配置文件）
	var common *v1.ClientCommonConfig
	var proxyCfgs []v1.ProxyConfigurer
	var visitorCfgs []v1.VisitorConfigurer

	if cfgFile != "" {
		result, err := config.LoadClientConfigResult(cfgFile, strictConfigMode)
		if err == nil {
			common = result.Common
			proxyCfgs = result.Proxies
			visitorCfgs = result.Visitors
		}
	}
	if common == nil {
		common = &v1.ClientCommonConfig{}
		if err := common.Complete(); err != nil {
			return fmt.Errorf("failed to complete default config: %v", err)
		}
	}

	// CLI 参数覆盖配置文件
	if applyServer != "" {
		common.ServerAddr = applyServer
	}
	if applyServerPort != 0 {
		common.ServerPort = applyServerPort
	}
	if applyToken != "" {
		common.Auth.Token = applyToken
	}

	// 确保必要字段有默认值
	if common.ServerAddr == "" {
		common.ServerAddr = "127.0.0.1"
	}
	if common.ServerPort == 0 {
		common.ServerPort = 7000
	}

	// Web 管理面板参数
	if applyWebPort > 0 {
		common.WebServer.Port = applyWebPort
		common.WebServer.Addr = applyWebAddr
		common.WebServer.User = applyWebUser
		common.WebServer.Password = applyWebPwd
	}

	// 构建 ConfigSource 和 Aggregator（apply 模式不需要任何 proxy/visitor）
	configSource := source.NewConfigSource()
	if err := configSource.ReplaceAll(proxyCfgs, visitorCfgs); err != nil {
		return fmt.Errorf("failed to init config source: %v", err)
	}
	aggregator := source.NewAggregator(configSource)

	log.InitLogger(common.Log.To, common.Log.Level, int(common.Log.MaxDays), common.Log.DisablePrintColor)

	svr, err := client.NewService(client.ServiceOptions{
		Common:                 common,
		ConfigSourceAggregator: aggregator,
	})
	if err != nil {
		return fmt.Errorf("failed to create service: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 在 goroutine 中运行 service（Run 是阻塞的）
	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- svr.Run(ctx)
	}()

	// 等待连接建立或 service 退出
	connectedCh := svr.WaitConnected()
	select {
	case <-connectedCh:
		// 连接成功，继续
	case err := <-runErrCh:
		return fmt.Errorf("service exited before connecting: %v", err)
	}

	// 提交申请
	code, err := svr.SubmitApplication(applyPorts, applyTypes, applyRemark)
	if err != nil {
		cancel()
		return fmt.Errorf("failed to submit application: %v", err)
	}

	outputApplyEvent("applied", map[string]interface{}{
		"code":   code,
		"ports":  applyPorts,
		"types":  applyTypes,
		"remark": applyRemark,
	})

	// 等待信号退出
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigCh:
		outputApplyEvent("stopped", nil)
		cancel()
	case err := <-runErrCh:
		if err != nil {
			outputApplyEvent("error", map[string]interface{}{"error": err.Error()})
			return err
		}
	case <-ctx.Done():
	}

	return nil
}

func outputApplyEvent(event string, data interface{}) {
	evt := map[string]interface{}{
		"event": event,
	}
	if data != nil {
		switch m := data.(type) {
		case map[string]interface{}:
			for k, v := range m {
				evt[k] = v
			}
		case map[string]string:
			for k, v := range m {
				evt[k] = v
			}
		}
	}
	b, _ := json.Marshal(evt)
	fmt.Fprintln(os.Stdout, string(b))
}
