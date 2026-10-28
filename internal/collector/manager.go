package collector

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"logcollector/internal/model"
	"logcollector/internal/sshx"

	"github.com/pkg/sftp"
)

const (
	connectTimeout = 20 * time.Second
	maxLogEntries  = 3000
	maxLogBytes    = 32 * 1024
)

type runSlot struct {
	state         model.RunState
	logs          []model.LogEntry
	cancel        context.CancelFunc
	running       bool
	stopRequested bool
	seq           int64
}

type Manager struct {
	baseCtx context.Context
	emit    func(model.Event)
	mu      sync.Mutex
	runs    map[string]*runSlot
}

func NewManager(baseCtx context.Context, emit func(model.Event)) *Manager {
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	return &Manager{
		baseCtx: baseCtx,
		emit:    emit,
		runs:    make(map[string]*runSlot),
	}
}

func newDeviceRunState(role string, device model.DeviceConfig) model.DeviceRunState {
	state := model.DeviceRunState{
		Role:    role,
		IP:      strings.TrimSpace(device.IP),
		Phase:   model.DeviceIdle,
		Message: "等待采集",
	}
	if role == "B" && strings.TrimSpace(device.IP) == "" {
		state.Phase = model.DeviceSkipped
		state.Message = "未配置设备 B，本次跳过"
	}
	return state
}

func newRunState(combo model.Combination, phase, message string) model.RunState {
	state := model.RunState{
		ComboID:   combo.ID,
		ComboName: combo.Name,
		Phase:     phase,
		Message:   message,
		Devices: []model.DeviceRunState{
			newDeviceRunState("A", combo.DeviceA),
			newDeviceRunState("B", combo.DeviceB),
		},
		LocalFiles: []string{},
	}
	if phase == model.RunRunning {
		state.StartedAt = nowString()
	}
	return state
}

// Idle returns the last runtime snapshot for a combination, or creates an idle
// one. Runtime history is intentionally in-memory only; configuration is the
// only data persisted between launches.
func (m *Manager) Idle(combo model.Combination) model.RunState {
	m.mu.Lock()
	if slot, ok := m.runs[combo.ID]; ok {
		state := cloneRunState(slot.state)
		m.mu.Unlock()
		return state
	}
	state := newRunState(combo, model.RunIdle, "尚未开始")
	m.runs[combo.ID] = &runSlot{state: state, logs: []model.LogEntry{}}
	m.mu.Unlock()
	return cloneRunState(state)
}

func (m *Manager) Start(cfg model.AppConfig, combo model.Combination) error {
	if err := combo.Validate(); err != nil {
		return err
	}
	m.mu.Lock()
	if err := m.baseCtx.Err(); err != nil {
		m.mu.Unlock()
		return errors.New("程序正在退出，无法开始采集")
	}
	if slot, ok := m.runs[combo.ID]; ok && slot.running {
		m.mu.Unlock()
		return errors.New("该组合正在采集中")
	}
	timeout := time.Duration(cfg.TimeoutMinutes) * time.Minute
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(m.baseCtx, timeout)
	slot := &runSlot{
		state:   newRunState(combo, model.RunRunning, "准备连接云主机"),
		logs:    []model.LogEntry{},
		cancel:  cancel,
		running: true,
		seq:     0,
	}
	m.runs[combo.ID] = slot
	state := cloneRunState(slot.state)
	m.mu.Unlock()

	m.emitRun(state)
	m.addLog(combo.ID, "", model.LogInfo, fmt.Sprintf("开始采集「%s」，云主机 %s", combo.Name, combo.Cloud.IP))
	m.addLog(combo.ID, "", model.LogInfo, fmt.Sprintf("执行流程：本地 → 云主机 %s → 设备 A（必需）→ 设备 B（可选，经设备 A 跳转）", combo.Cloud.IP))

	go func() {
		defer cancel()
		m.execute(ctx, cfg, combo)
	}()
	return nil
}

func (m *Manager) Stop(comboID string) error {
	active := m.requestStop(comboID)
	if !active {
		return errors.New("该组合当前未在采集")
	}
	m.addLog(comboID, "", model.LogWarn, "已请求停止采集，正在终止设备上的命令…")
	return nil
}

func (m *Manager) requestStop(comboID string) bool {
	m.mu.Lock()
	slot, ok := m.runs[comboID]
	if !ok || !slot.running {
		m.mu.Unlock()
		return false
	}
	slot.stopRequested = true
	slot.state.Message = "正在停止，等待设备响应…"
	for i := range slot.state.Devices {
		switch slot.state.Devices[i].Phase {
		case model.DeviceCompleted, model.DeviceFailed, model.DeviceStopped, model.DeviceSkipped:
		default:
			slot.state.Devices[i].Message = "正在停止…"
		}
	}
	if slot.cancel != nil {
		slot.cancel()
	}
	state := cloneRunState(slot.state)
	m.mu.Unlock()
	m.emitRun(state)
	return true
}

func (m *Manager) StopAll() int {
	m.mu.Lock()
	ids := make([]string, 0)
	for id, slot := range m.runs {
		if slot.running {
			ids = append(ids, id)
		}
	}
	m.mu.Unlock()
	count := 0
	for _, id := range ids {
		if m.requestStop(id) {
			count++
			m.addLog(id, "", model.LogWarn, "已请求停止采集，正在终止设备上的命令…")
		}
	}
	return count
}

func (m *Manager) execute(ctx context.Context, cfg model.AppConfig, combo model.Combination) {
	defer m.finish(combo.ID, ctx)

	setRunMessage := func(message string) {
		m.updateRun(combo.ID, func(slot *runSlot) { slot.state.Message = message })
	}
	failDevice := func(role, message string) {
		m.updateDevice(combo.ID, role, func(d *model.DeviceRunState) {
			d.Phase = model.DeviceFailed
			d.Message = message
			d.FinishedAt = nowString()
		})
	}

	cloudAddr := model.JoinHostPort(combo.Cloud.IP, combo.Cloud.Port)
	setRunMessage("正在连接云主机 " + cloudAddr)
	m.addLog(combo.ID, "", model.LogInfo, fmt.Sprintf("连接云主机 %s（用户 %s）", cloudAddr, combo.Cloud.Username))
	cloud, err := sshx.Dial(ctx, cloudAddr, combo.Cloud.Username, combo.Cloud.Password, connectTimeout)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		msg := "云主机连接失败：" + friendlySSHError(err)
		m.addLog(combo.ID, "", model.LogError, msg)
		failDevice("A", "云主机连接失败，无法连接设备")
		if strings.TrimSpace(combo.DeviceB.IP) != "" {
			failDevice("B", "云主机连接失败，无法连接设备")
		}
		setRunMessage(msg)
		return
	}
	defer cloud.Close()
	m.addLog(combo.ID, "", model.LogSuccess, "云主机 SSH 已连接")

	cloudSFTP, err := sftp.NewClient(cloud.Client)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		msg := "云主机 SFTP 初始化失败：" + err.Error()
		m.addLog(combo.ID, "", model.LogError, msg)
		failDevice("A", "云主机不支持文件传输，采集终止")
		if strings.TrimSpace(combo.DeviceB.IP) != "" {
			failDevice("B", "云主机不支持文件传输，采集终止")
		}
		setRunMessage(msg)
		return
	}
	defer cloudSFTP.Close()

	// Device A is mandatory and doubles as the jump host for device B.
	deviceAAddr := model.JoinHostPort(combo.DeviceA.IP, combo.DeviceA.Port)
	m.updateDevice(combo.ID, "A", func(d *model.DeviceRunState) {
		d.Phase = model.DeviceConnecting
		d.Message = "经云主机建立 SSH 通道"
		d.StartedAt = nowString()
	})
	setRunMessage("正在连接设备 A " + deviceAAddr)
	m.addLog(combo.ID, "A", model.LogInfo, fmt.Sprintf("经云主机连接设备 A %s（用户 %s）", deviceAAddr, combo.DeviceA.Username))
	deviceA, err := sshx.DialVia(ctx, cloud, deviceAAddr, combo.DeviceA.Username, combo.DeviceA.Password, connectTimeout)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		msg := "设备 A 连接失败：" + friendlySSHError(err)
		m.addLog(combo.ID, "A", model.LogError, msg)
		failDevice("A", msg)
		if strings.TrimSpace(combo.DeviceB.IP) != "" {
			bmsg := "设备 A 连接失败，无法经 A 跳转到设备 B"
			m.addLog(combo.ID, "B", model.LogError, bmsg)
			failDevice("B", bmsg)
		}
		setRunMessage(msg)
		return
	}
	defer deviceA.Close()
	m.updateDevice(combo.ID, "A", func(d *model.DeviceRunState) {
		d.Message = "SSH 已连接"
	})
	m.addLog(combo.ID, "A", model.LogSuccess, "设备 A SSH 已连接")

	var deviceB *sshx.Client
	bEnabled := strings.TrimSpace(combo.DeviceB.IP) != ""
	if bEnabled {
		deviceBAddr := model.JoinHostPort(combo.DeviceB.IP, combo.DeviceB.Port)
		m.updateDevice(combo.ID, "B", func(d *model.DeviceRunState) {
			d.Phase = model.DeviceConnecting
			d.Message = "经设备 A 跳转"
			d.StartedAt = nowString()
		})
		setRunMessage("正在经设备 A 连接设备 B " + deviceBAddr)
		m.addLog(combo.ID, "B", model.LogInfo, fmt.Sprintf("经设备 A 跳转连接设备 B %s（用户 %s）", deviceBAddr, combo.DeviceB.Username))
		deviceB, err = sshx.DialVia(ctx, deviceA, deviceBAddr, combo.DeviceB.Username, combo.DeviceB.Password, connectTimeout)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			msg := "设备 B 连接失败：" + friendlySSHError(err)
			m.addLog(combo.ID, "B", model.LogError, msg+". 将继续采集设备 A。")
			failDevice("B", msg)
			deviceB = nil
		} else {
			defer deviceB.Close()
			m.updateDevice(combo.ID, "B", func(d *model.DeviceRunState) {
				d.Message = "SSH 已连接"
			})
			m.addLog(combo.ID, "B", model.LogSuccess, "设备 B SSH 已通过设备 A 跳转连接")
		}
	}

	if ctx.Err() != nil {
		return
	}
	setRunMessage("正在采集设备 A")
	if err := m.processDevice(ctx, cfg, combo, "A", combo.DeviceA, deviceA, cloudSFTP); err != nil {
		if ctx.Err() != nil {
			return
		}
		m.addLog(combo.ID, "A", model.LogError, "设备 A 处理失败："+err.Error())
		failDevice("A", err.Error())
	} else {
		m.addLog(combo.ID, "A", model.LogSuccess, "设备 A 全部完成")
	}

	if ctx.Err() != nil {
		return
	}
	if bEnabled && deviceB != nil {
		setRunMessage("正在采集设备 B")
		if err := m.processDevice(ctx, cfg, combo, "B", combo.DeviceB, deviceB, cloudSFTP); err != nil {
			if ctx.Err() != nil {
				return
			}
			m.addLog(combo.ID, "B", model.LogError, "设备 B 处理失败："+err.Error())
			failDevice("B", err.Error())
		} else {
			m.addLog(combo.ID, "B", model.LogSuccess, "设备 B 全部完成")
		}
	}
}

func (m *Manager) finish(comboID string, ctx context.Context) {
	now := nowString()
	m.mu.Lock()
	slot, ok := m.runs[comboID]
	if !ok || !slot.running {
		m.mu.Unlock()
		return
	}
	slot.running = false
	slot.cancel = nil
	stopRequested := slot.stopRequested
	slot.state.FinishedAt = now

	ctxErr := ctx.Err()
	timedOut := errors.Is(ctxErr, context.DeadlineExceeded)
	stopped := stopRequested || errors.Is(ctxErr, context.Canceled)

	enabled, completed, failed := 0, 0, 0
	for i := range slot.state.Devices {
		d := &slot.state.Devices[i]
		if d.Phase == model.DeviceSkipped {
			continue
		}
		enabled++
		switch d.Phase {
		case model.DeviceCompleted:
			completed++
		case model.DeviceFailed:
			failed++
		}
	}
	// Devices that could not finish receive a terminal state before the run
	// itself is finalized, so the UI never shows a running device in a stopped
	// run.
	for i := range slot.state.Devices {
		d := &slot.state.Devices[i]
		if d.Phase == model.DeviceCompleted || d.Phase == model.DeviceFailed ||
			d.Phase == model.DeviceStopped || d.Phase == model.DeviceSkipped {
			continue
		}
		failed++
		switch {
		case stopped:
			d.Phase = model.DeviceStopped
			d.Message = "已停止"
		case timedOut:
			d.Phase = model.DeviceFailed
			d.Message = "采集超时"
		default:
			d.Phase = model.DeviceFailed
			d.Message = "未完成"
		}
		d.FinishedAt = now
	}

	switch {
	case enabled > 0 && completed == enabled:
		slot.state.Phase = model.RunSuccess
		slot.state.Message = fmt.Sprintf("%d 台设备全部完成", enabled)
		if enabled == 1 {
			slot.state.Message = "设备 A 已完成"
		}
	case stopped:
		slot.state.Phase = model.RunStopped
		slot.state.Message = "采集已停止"
	case timedOut:
		slot.state.Phase = model.RunFailed
		slot.state.Message = "采集超时，请检查设备网络或增大超时时间"
	case completed > 0:
		slot.state.Phase = model.RunPartial
		slot.state.Message = fmt.Sprintf("部分完成：%d 成功，%d 失败", completed, failed)
	default:
		slot.state.Phase = model.RunFailed
		slot.state.Message = "采集失败，请查看错误日志"
	}
	state := cloneRunState(slot.state)
	m.mu.Unlock()

	m.emitRun(state)
	level := model.LogSuccess
	switch state.Phase {
	case model.RunSuccess:
		level = model.LogSuccess
	case model.RunPartial:
		level = model.LogWarn
	case model.RunStopped:
		level = model.LogWarn
	default:
		level = model.LogError
	}
	m.addLog(comboID, "", level, "本次采集结束："+state.Message)
}

func (m *Manager) processDevice(
	ctx context.Context,
	cfg model.AppConfig,
	combo model.Combination,
	role string,
	device model.DeviceConfig,
	client *sshx.Client,
	cloudSFTP *sftp.Client,
) error {
	if client == nil {
		return errors.New("设备连接不存在")
	}
	m.updateDevice(combo.ID, role, func(d *model.DeviceRunState) {
		d.Phase = model.DeviceCollecting
		d.Message = "正在执行采集命令"
		if d.StartedAt == "" {
			d.StartedAt = nowString()
		}
	})
	m.updateRun(combo.ID, func(slot *runSlot) {
		slot.state.Message = fmt.Sprintf("设备 %s：正在执行采集命令", role)
	})
	m.addLog(combo.ID, role, model.LogInfo, "开始执行采集命令，等待结束标记…")

	sink := func(stream, line string) {
		if line == "" {
			return
		}
		level := model.LogOutput
		if stream == "stderr" {
			level = model.LogWarn
		}
		m.addLog(combo.ID, role, level, line)
	}
	collectionCmd := BuildCollectionCommand(device.Commands, device.EndMarker)
	result, err := runRemoteCommand(ctx, client.Client, collectionCmd, device.EndMarker, sink)
	if err != nil {
		return err
	}
	if !result.MarkerSeen {
		return fmt.Errorf("未检测到结束标记 %q；无法确认采集完成", device.EndMarker)
	}
	if result.ExitStatus != 0 {
		m.addLog(combo.ID, role, model.LogWarn, fmt.Sprintf("采集命令已结束，但退出码为 %d；将继续打包已有日志", result.ExitStatus))
	}
	m.addLog(combo.ID, role, model.LogSuccess, fmt.Sprintf("检测到结束标记 %s（退出码 %d），采集阶段完成", device.EndMarker, result.ExitStatus))
	if err := ctx.Err(); err != nil {
		return err
	}

	archiveName := ExpandArchiveName(cfg.ArchiveNameTemplate, combo, role)
	archivePath := pathJoin(cfg.DeviceTempDir, archiveName)
	m.updateDevice(combo.ID, role, func(d *model.DeviceRunState) {
		d.Phase = model.DeviceArchiving
		d.Message = "正在打包日志"
		d.ArchiveName = archiveName
	})
	m.updateRun(combo.ID, func(slot *runSlot) {
		slot.state.Message = fmt.Sprintf("设备 %s：正在打包日志", role)
	})
	m.addLog(combo.ID, role, model.LogInfo, fmt.Sprintf("打包日志：%s", strings.Join(device.CollectPaths, ", ")))
	archiveCmd := BuildArchiveCommand(cfg.DeviceTempDir, archivePath, device.CollectPaths)
	archiveResult, err := runRemoteCommand(ctx, client.Client, archiveCmd, "", sink)
	if err != nil {
		return fmt.Errorf("打包日志失败：%w", err)
	}
	if archiveResult.ExitStatus != 0 {
		return fmt.Errorf("打包日志失败，tar 退出码 %d", archiveResult.ExitStatus)
	}

	deviceSFTP, err := sftp.NewClient(client.Client)
	if err != nil {
		return fmt.Errorf("设备 SFTP 初始化失败：%w", err)
	}
	defer deviceSFTP.Close()

	archiveInfo, err := deviceSFTP.Stat(archivePath)
	if err != nil {
		return fmt.Errorf("读取设备归档失败：%w", err)
	}
	if archiveInfo.IsDir() || archiveInfo.Size() == 0 {
		return fmt.Errorf("设备归档为空或无效：%s", archivePath)
	}

	cloudFolder := SafeComboDir(combo.Name, combo.ID)
	cloudPath := pathJoin(cfg.CloudStageDir, cloudFolder, archiveName)
	m.updateDevice(combo.ID, role, func(d *model.DeviceRunState) {
		d.Phase = model.DeviceUploading
		d.Message = "正在传输到云主机"
		d.CloudPath = cloudPath
	})
	m.updateRun(combo.ID, func(slot *runSlot) {
		slot.state.Message = fmt.Sprintf("设备 %s：正在传输到云主机", role)
	})
	m.addLog(combo.ID, role, model.LogInfo, fmt.Sprintf("传输设备 → 云主机：%s（%s）", cloudPath, FormatBytes(archiveInfo.Size())))
	transferred, err := copyDeviceToCloud(ctx, deviceSFTP, cloudSFTP, archivePath, cloudPath)
	if err != nil {
		return err
	}
	m.addLog(combo.ID, role, model.LogSuccess, fmt.Sprintf("已传输到云主机：%s", FormatBytes(transferred)))

	if err := removeRemoteFile(deviceSFTP, archivePath); err != nil {
		m.addLog(combo.ID, role, model.LogWarn, "清理设备临时归档失败："+err.Error())
	}

	m.updateDevice(combo.ID, role, func(d *model.DeviceRunState) {
		d.Phase = model.DeviceDownloading
		d.Message = "正在下载到本地"
		d.Bytes = transferred
	})
	m.updateRun(combo.ID, func(slot *runSlot) {
		slot.state.Message = fmt.Sprintf("设备 %s：正在下载到本地", role)
	})
	downloadRoot, err := expandDownloadRoot(cfg.DownloadDir)
	if err != nil {
		return err
	}
	localDir := pathJoinLocal(downloadRoot, cloudFolder)
	m.addLog(combo.ID, role, model.LogInfo, fmt.Sprintf("传输云主机 → 本地：%s", localDir))
	localPath, downloaded, err := copyCloudToLocal(ctx, cloudSFTP, cloudPath, localDir, archiveName)
	if err != nil {
		return err
	}
	m.addLog(combo.ID, role, model.LogSuccess, fmt.Sprintf("日志已保存：%s（%s）", localPath, FormatBytes(downloaded)))

	if !cfg.KeepCloudCopy {
		if err := removeRemoteFile(cloudSFTP, cloudPath); err != nil {
			m.addLog(combo.ID, role, model.LogWarn, "清理云主机暂存归档失败："+err.Error())
		} else {
			m.addLog(combo.ID, role, model.LogInfo, "已清理云主机暂存归档")
		}
	} else {
		m.addLog(combo.ID, role, model.LogInfo, "已保留云主机暂存归档："+cloudPath)
	}

	m.updateDevice(combo.ID, role, func(d *model.DeviceRunState) {
		d.Phase = model.DeviceCompleted
		d.Message = "采集、传输、下载完成"
		d.CloudPath = cloudPath
		d.LocalPath = localPath
		d.Bytes = downloaded
		d.FinishedAt = nowString()
	})
	m.updateRun(combo.ID, func(slot *runSlot) {
		slot.state.LocalFiles = appendUnique(slot.state.LocalFiles, localPath)
	})
	return nil
}

func (m *Manager) updateRun(id string, fn func(*runSlot)) {
	m.mu.Lock()
	slot, ok := m.runs[id]
	if !ok {
		m.mu.Unlock()
		return
	}
	fn(slot)
	state := cloneRunState(slot.state)
	m.mu.Unlock()
	m.emitRun(state)
}

func (m *Manager) updateDevice(id, role string, fn func(*model.DeviceRunState)) {
	m.updateRun(id, func(slot *runSlot) {
		for i := range slot.state.Devices {
			if slot.state.Devices[i].Role == role {
				fn(&slot.state.Devices[i])
				return
			}
		}
	})
}

func (m *Manager) addLog(comboID, device, level, message string) {
	message = strings.TrimRight(message, "\r\n")
	if strings.TrimSpace(message) == "" {
		return
	}
	if len(message) > maxLogBytes {
		runes := []rune(message)
		if len(runes) > 8000 {
			message = string(runes[:8000]) + " …[已截断]"
		} else {
			message = message[:maxLogBytes] + " …[已截断]"
		}
	}
	m.mu.Lock()
	slot, ok := m.runs[comboID]
	if !ok {
		m.mu.Unlock()
		return
	}
	slot.seq++
	entry := model.LogEntry{
		Seq:     slot.seq,
		ComboID: comboID,
		Device:  device,
		Level:   level,
		Message: message,
		At:      nowString(),
	}
	slot.logs = append(slot.logs, entry)
	if len(slot.logs) > maxLogEntries {
		slot.logs = append([]model.LogEntry(nil), slot.logs[len(slot.logs)-maxLogEntries:]...)
	}
	m.mu.Unlock()
	if m.emit != nil {
		entryCopy := entry
		m.emit(model.Event{Kind: "log", Log: &entryCopy})
	}
}

func (m *Manager) emitRun(state model.RunState) {
	if m.emit == nil {
		return
	}
	stateCopy := cloneRunState(state)
	m.emit(model.Event{Kind: "run", Run: &stateCopy})
}

func (m *Manager) Logs(comboID string) []model.LogEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	slot, ok := m.runs[comboID]
	if !ok {
		return []model.LogEntry{}
	}
	return append([]model.LogEntry(nil), slot.logs...)
}

func (m *Manager) ClearLogs(comboID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if slot, ok := m.runs[comboID]; ok {
		slot.logs = nil
	}
}

func (m *Manager) IsRunning(comboID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	slot, ok := m.runs[comboID]
	return ok && slot.running
}

func cloneRunState(state model.RunState) model.RunState {
	out := state
	out.Devices = append([]model.DeviceRunState(nil), state.Devices...)
	out.LocalFiles = append([]string(nil), state.LocalFiles...)
	if out.Devices == nil {
		out.Devices = []model.DeviceRunState{}
	}
	if out.LocalFiles == nil {
		out.LocalFiles = []string{}
	}
	return out
}

func appendUnique(items []string, item string) []string {
	for _, existing := range items {
		if existing == item {
			return items
		}
	}
	return append(items, item)
}

func nowString() string {
	return time.Now().Format(time.RFC3339)
}

func friendlySSHError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if strings.Contains(msg, "unable to authenticate") || strings.Contains(msg, "no supported methods remain") {
		return "认证失败，请检查用户名和密码（" + msg + "）"
	}
	if strings.Contains(msg, "i/o timeout") || strings.Contains(msg, "context deadline exceeded") {
		return "连接超时，请检查 IP、端口和网络可达性（" + msg + "）"
	}
	if strings.Contains(msg, "connection refused") {
		return "连接被拒绝，请检查 SSH 端口（" + msg + "）"
	}
	if strings.Contains(msg, "no common algorithm") || strings.Contains(msg, "handshake failed") {
		return "SSH 握手失败，设备可能使用了不兼容的加密算法（" + msg + "）"
	}
	return msg
}
