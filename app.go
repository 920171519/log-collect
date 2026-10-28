package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"

	"logcollector/internal/collector"
	"logcollector/internal/model"
	"logcollector/internal/store"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is exposed to the Wails frontend. All operations involving SSH and files
// live in internal packages; this type is the thin application boundary.
type App struct {
	ctx     context.Context
	mu      sync.Mutex
	store   *store.Store
	manager *collector.Manager
	initErr string
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.mu.Lock()
	a.ctx = ctx
	a.mu.Unlock()

	st, err := store.Open("")
	if err != nil {
		a.mu.Lock()
		a.initErr = "初始化配置文件失败：" + err.Error()
		a.mu.Unlock()
		return
	}
	manager := collector.NewManager(ctx, func(event model.Event) {
		select {
		case <-ctx.Done():
			return
		default:
			wailsruntime.EventsEmit(ctx, "collector:event", event)
		}
	})
	a.mu.Lock()
	a.store = st
	a.manager = manager
	a.initErr = ""
	if loadErr := st.LoadError(); loadErr != "" {
		a.initErr = loadErr
	}
	a.mu.Unlock()
}

func (a *App) shutdown(ctx context.Context) {
	a.mu.Lock()
	manager := a.manager
	a.mu.Unlock()
	if manager != nil {
		manager.StopAll()
	}
}

func (a *App) GetAppState() model.AppState {
	a.mu.Lock()
	st := a.store
	manager := a.manager
	initErr := a.initErr
	a.mu.Unlock()

	if st == nil {
		cfg := model.DefaultConfig()
		cfg.DownloadDir = store.DefaultDownloadDir()
		return model.AppState{
			Config:      cfg,
			ConfigPath:  "",
			ConfigError: initErr,
			Runs:        []model.RunState{},
		}
	}
	cfg := st.Get()
	runs := make([]model.RunState, 0, len(cfg.Combinations))
	if manager != nil {
		for _, combo := range cfg.Combinations {
			runs = append(runs, manager.Idle(combo))
		}
	}
	configError := initErr
	if configError == "" {
		configError = st.LoadError()
	}
	return model.AppState{
		Config:      cfg,
		ConfigPath:  st.Path(),
		ConfigError: configError,
		Runs:        runs,
	}
}

// NewCombinationDraft returns a fresh, fully editable combination with the
// same example recipe used by freshly created configuration files.
func (a *App) NewCombinationDraft() model.Combination {
	return model.NewCombination("")
}

func (a *App) SaveConfig(cfg model.AppConfig) (model.AppConfig, error) {
	a.mu.Lock()
	st := a.store
	a.mu.Unlock()
	if st == nil {
		return model.AppConfig{}, errors.New("配置文件尚未初始化")
	}
	saved, err := st.Save(cfg)
	if err != nil {
		return model.AppConfig{}, err
	}
	a.mu.Lock()
	a.initErr = ""
	a.mu.Unlock()
	return saved, nil
}

func (a *App) StartCombination(comboID string) error {
	a.mu.Lock()
	st := a.store
	manager := a.manager
	a.mu.Unlock()
	if st == nil || manager == nil {
		return errors.New("应用尚未初始化")
	}
	cfg := st.Get()
	combo, ok := findCombo(cfg, comboID)
	if !ok {
		return errors.New("找不到该云主机组合，请先保存配置")
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := combo.Validate(); err != nil {
		return err
	}
	return manager.Start(cfg, combo)
}

func (a *App) StartAll() (int, error) {
	a.mu.Lock()
	st := a.store
	manager := a.manager
	a.mu.Unlock()
	if st == nil || manager == nil {
		return 0, errors.New("应用尚未初始化")
	}
	cfg := st.Get()
	if err := cfg.Validate(); err != nil {
		return 0, err
	}
	if len(cfg.Combinations) == 0 {
		return 0, errors.New("还没有云主机组合，请先新增并保存")
	}
	started, invalid := 0, 0
	var lastErr error
	for _, combo := range cfg.Combinations {
		if err := combo.Validate(); err != nil {
			invalid++
			continue
		}
		if manager.IsRunning(combo.ID) {
			continue
		}
		if err := manager.Start(cfg, combo); err != nil {
			lastErr = err
			continue
		}
		started++
	}
	if started == 0 {
		if invalid > 0 {
			return 0, fmt.Errorf("没有可启动的组合：%d 个组合缺少 IP、账号、密码或采集命令", invalid)
		}
		if lastErr != nil {
			return 0, lastErr
		}
		return 0, errors.New("没有需要启动的组合（可能已在采集中）")
	}
	if invalid > 0 {
		return started, fmt.Errorf("已启动 %d 个组合；%d 个组合信息不完整，已跳过", started, invalid)
	}
	return started, nil
}

func (a *App) StopCombination(comboID string) error {
	a.mu.Lock()
	manager := a.manager
	a.mu.Unlock()
	if manager == nil {
		return errors.New("应用尚未初始化")
	}
	return manager.Stop(comboID)
}

func (a *App) StopAll() (int, error) {
	a.mu.Lock()
	manager := a.manager
	a.mu.Unlock()
	if manager == nil {
		return 0, errors.New("应用尚未初始化")
	}
	count := manager.StopAll()
	if count == 0 {
		return 0, errors.New("当前没有正在运行的采集任务")
	}
	return count, nil
}

func (a *App) GetLogs(comboID string) []model.LogEntry {
	a.mu.Lock()
	manager := a.manager
	a.mu.Unlock()
	if manager == nil {
		return []model.LogEntry{}
	}
	return manager.Logs(comboID)
}

func (a *App) ClearLogs(comboID string) {
	a.mu.Lock()
	manager := a.manager
	a.mu.Unlock()
	if manager != nil {
		manager.ClearLogs(comboID)
	}
}

func (a *App) ChooseDownloadDir(current string) (string, error) {
	a.mu.Lock()
	ctx := a.ctx
	a.mu.Unlock()
	if ctx == nil {
		return "", errors.New("应用尚未初始化")
	}
	opts := wailsruntime.OpenDialogOptions{
		Title:                "选择日志下载目录",
		DefaultDirectory:     current,
		CanCreateDirectories: true,
	}
	return wailsruntime.OpenDirectoryDialog(ctx, opts)
}

func (a *App) OpenDownloadFolder() error {
	a.mu.Lock()
	st := a.store
	a.mu.Unlock()
	if st == nil {
		return errors.New("应用尚未初始化")
	}
	cfg := st.Get()
	dir := cfg.DownloadDir
	if strings.TrimSpace(dir) == "" {
		dir = store.DefaultDownloadDir()
	}
	if strings.HasPrefix(dir, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			dir = filepath.Join(home, strings.TrimPrefix(dir, "~/"))
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("创建下载目录失败：%w", err)
	}
	return openLocalPath(dir)
}

func (a *App) OpenConfigFolder() error {
	a.mu.Lock()
	st := a.store
	a.mu.Unlock()
	if st == nil {
		return errors.New("配置文件尚未初始化")
	}
	return openLocalPath(filepath.Dir(st.Path()))
}

func findCombo(cfg model.AppConfig, id string) (model.Combination, bool) {
	for _, combo := range cfg.Combinations {
		if combo.ID == id {
			return combo, true
		}
	}
	return model.Combination{}, false
}

func openLocalPath(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("路径为空")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, err := os.Stat(abs); err != nil {
		return fmt.Errorf("路径不存在：%w", err)
	}
	var cmd *exec.Cmd
	switch goruntime.GOOS {
	case "darwin":
		cmd = exec.Command("open", abs)
	case "windows":
		cmd = exec.Command("explorer", abs)
	default:
		cmd = exec.Command("xdg-open", abs)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("打开系统文件管理器失败：%w", err)
	}
	return nil
}
