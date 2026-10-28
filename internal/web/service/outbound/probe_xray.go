package outbound

import (
	"context"
	"encoding/json"
	"os"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
	"github.com/SawaMEN/3x-ui/v3/internal/xray"
)

type xrayBatchProcess struct {
	process *singbox.Process
	config  *xray.Config
	path    string
}

func newXrayBatchProcess(cfg *xray.Config, path string) batchProcess {
	return &xrayBatchProcess{process: singbox.NewProcessWithOptions(path, singbox.ProcessOptions{
		Name: "Xray", Binary: config.StandaloneCoreBinaryPath("xray"), RunCommand: "run", VersionPrefix: "Xray ", Xray: true,
	}, true), config: cfg, path: path}
}

func (p *xrayBatchProcess) Start() error {
	data, err := json.Marshal(p.config)
	if err != nil {
		return err
	}
	if err := os.WriteFile(p.path, data, 0o600); err != nil {
		return err
	}
	return p.process.Start(context.Background())
}
func (p *xrayBatchProcess) Stop() error     { return p.process.Stop() }
func (p *xrayBatchProcess) IsRunning() bool { return p.process.IsRunning() }
func (p *xrayBatchProcess) GetResult() string {
	if err := p.process.GetErr(); err != nil {
		return err.Error()
	}
	return ""
}
