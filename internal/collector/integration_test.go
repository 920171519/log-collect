package collector

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"logcollector/internal/model"
)

type testTopology struct {
	cloud *testSSHServer
	a     *testSSHServer
	b     *testSSHServer

	config *model.AppConfig
	combo  model.Combination
}

func newTestTopology(t *testing.T, withB bool) *testTopology {
	t.Helper()
	signer := newED25519Signer(t)
	cloud := startTestSSHServer(t, "cloud", "cloud-secret", signer)
	a := startTestSSHServer(t, "device-a", "device-a-secret", signer)
	b := startTestSSHServer(t, "device-b", "device-b-secret", signer)

	base := t.TempDir()
	cloudStage := filepath.Join(base, "cloud-stage")
	deviceTemp := filepath.Join(base, "device-temp")
	downloadDir := filepath.Join(base, "downloads")
	aLogs := filepath.Join(base, "a-logs")
	bLogs := filepath.Join(base, "b-logs")

	cfg := model.DefaultConfig()
	cfg.DownloadDir = downloadDir
	cfg.ArchiveNameTemplate = "test-{device}.tar.gz"
	cfg.CloudStageDir = cloudStage
	cfg.DeviceTempDir = deviceTemp
	cfg.KeepCloudCopy = false
	cfg.TimeoutMinutes = 1

	combo := model.NewCombination("测试组合")
	combo.ID = "combo-test"
	combo.Cloud = model.HostConfig{
		IP:       "127.0.0.1",
		Port:     cloud.port(),
		Username: "root",
		Password: "cloud-secret",
	}
	combo.DeviceA = model.DefaultDevice("A")
	combo.DeviceA.IP = "127.0.0.1"
	combo.DeviceA.Port = a.port()
	combo.DeviceA.Username = "root"
	combo.DeviceA.Password = "device-a-secret"
	combo.DeviceA.CollectPaths = []string{aLogs}
	combo.DeviceA.Commands = "mkdir -p " + ShellQuote(aLogs) + "\n" +
		"printf 'A-HOST\\n' > " + ShellQuote(filepath.Join(aLogs, "host.txt")) + "\n" +
		"printf 'stderr-from-a\\n' >&2\n"

	combo.DeviceB = model.DefaultDevice("B")
	combo.DeviceB.IP = "127.0.0.1"
	combo.DeviceB.Port = b.port()
	combo.DeviceB.Username = "root"
	combo.DeviceB.Password = "device-b-secret"
	combo.DeviceB.CollectPaths = []string{bLogs}
	combo.DeviceB.Commands = "mkdir -p " + ShellQuote(bLogs) + "\n" +
		"printf 'B-HOST\\n' > " + ShellQuote(filepath.Join(bLogs, "host.txt")) + "\n"
	if !withB {
		combo.DeviceB = model.DefaultDevice("B")
		combo.DeviceB.IP = ""
	}
	cfg.Combinations = []model.Combination{combo}
	return &testTopology{cloud: cloud, a: a, b: b, config: &cfg, combo: combo}
}

func isTerminalPhase(phase model.RunState) bool {
	switch phase.Phase {
	case model.RunSuccess, model.RunPartial, model.RunFailed, model.RunStopped:
		return true
	default:
		return false
	}
}

func waitForTerminal(t *testing.T, manager *Manager, comboID string, timeout time.Duration) model.RunState {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		state := manager.Idle(comboModel(manager, comboID))
		if isTerminalPhase(state) {
			return state
		}
		time.Sleep(40 * time.Millisecond)
	}
	t.Fatalf("collection did not reach a terminal state within %s; logs:\n%s", timeout, formatLogs(manager.Logs(comboID)))
	return model.RunState{}
}

// comboModel reconstructs only the ID and IPs needed by Idle to read an
// existing runtime slot. Idle does not use the config for existing slots.
func comboModel(manager *Manager, comboID string) model.Combination {
	return model.Combination{ID: comboID}
}

func formatLogs(logs []model.LogEntry) string {
	var b strings.Builder
	for _, entry := range logs {
		b.WriteString(entry.At)
		b.WriteString(" [")
		b.WriteString(entry.Device)
		b.WriteString("] ")
		b.WriteString(entry.Level)
		b.WriteString(": ")
		b.WriteString(entry.Message)
		b.WriteString("\n")
	}
	return b.String()
}

func startTestManager(t *testing.T, cfg model.AppConfig, combo model.Combination) (*Manager, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	manager := NewManager(ctx, nil)
	if err := manager.Start(cfg, combo); err != nil {
		cancel()
		t.Fatalf("start collection: %v", err)
	}
	t.Cleanup(func() {
		manager.StopAll()
		cancel()
	})
	return manager, cancel
}

func collectArchiveText(t *testing.T, archivePath string) string {
	t.Helper()
	file, err := os.Open(archivePath)
	if err != nil {
		t.Fatalf("open archive %s: %v", archivePath, err)
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("open gzip %s: %v", archivePath, err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	var contents strings.Builder
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read tar %s: %v", archivePath, err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		data, err := io.ReadAll(tarReader)
		if err != nil {
			t.Fatalf("read tar entry %s: %v", header.Name, err)
		}
		contents.WriteString("### " + header.Name + "\n")
		contents.Write(data)
	}
	return contents.String()
}

func TestCollectorEndToEndThroughCloudAAndB(t *testing.T) {
	topology := newTestTopology(t, true)
	manager, _ := startTestManager(t, *topology.config, topology.combo)
	final := waitForTerminal(t, manager, topology.combo.ID, 20*time.Second)
	logs := formatLogs(manager.Logs(topology.combo.ID))

	if final.Phase != model.RunSuccess {
		t.Fatalf("expected success, got %s: %s\nlogs:\n%s", final.Phase, final.Message, logs)
	}
	if len(final.Devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(final.Devices))
	}
	devices := map[string]model.DeviceRunState{}
	for _, device := range final.Devices {
		devices[device.Role] = device
	}
	for _, role := range []string{"A", "B"} {
		device := devices[role]
		if device.Phase != model.DeviceCompleted {
			t.Fatalf("device %s expected completed, got %s: %s\nlogs:\n%s", role, device.Phase, device.Message, logs)
		}
		if device.LocalPath == "" {
			t.Fatalf("device %s has no local path", role)
		}
		if _, err := os.Stat(device.LocalPath); err != nil {
			t.Fatalf("local archive missing for %s: %v", role, err)
		}
		if device.Bytes <= 0 {
			t.Fatalf("device %s has non-positive byte count %d", role, device.Bytes)
		}
		if _, err := os.Stat(device.CloudPath); !os.IsNotExist(err) {
			t.Fatalf("cloud staging file should have been removed for %s: %v", role, err)
		}
		text := collectArchiveText(t, device.LocalPath)
		want := role + "-HOST"
		if !strings.Contains(text, want) {
			t.Fatalf("archive for %s does not contain %q:\n%s", role, want, text)
		}
	}

	if len(final.LocalFiles) != 2 {
		t.Fatalf("expected 2 local archives, got %d", len(final.LocalFiles))
	}
	if !strings.Contains(logs, "检测到结束标记") {
		t.Fatalf("logs do not report the completion marker:\n%s", logs)
	}
	if !strings.Contains(logs, "stderr-from-a") {
		t.Fatalf("stderr was not streamed into the logs:\n%s", logs)
	}

	cloudForwards := topology.cloud.forwardedTargets()
	if len(cloudForwards) == 0 || !strings.Contains(strings.Join(cloudForwards, ","), topology.a.addr()) {
		t.Fatalf("cloud did not forward to device A %s: %v", topology.a.addr(), cloudForwards)
	}
	aForwards := topology.a.forwardedTargets()
	if len(aForwards) == 0 || !strings.Contains(strings.Join(aForwards, ","), topology.b.addr()) {
		t.Fatalf("device A did not forward to device B %s: %v", topology.b.addr(), aForwards)
	}
	if topology.b.acceptCount() == 0 {
		t.Fatal("device B SSH server was never reached")
	}

	entries, err := os.ReadDir(topology.config.DeviceTempDir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read device temp dir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tar.gz") {
			t.Fatalf("device temporary archive was not cleaned up: %s", entry.Name())
		}
	}
}

func TestCollectorSkipsDeviceBWhenIPBlank(t *testing.T) {
	topology := newTestTopology(t, false)
	manager, _ := startTestManager(t, *topology.config, topology.combo)
	final := waitForTerminal(t, manager, topology.combo.ID, 20*time.Second)
	logs := formatLogs(manager.Logs(topology.combo.ID))
	if final.Phase != model.RunSuccess {
		t.Fatalf("expected success, got %s: %s\nlogs:\n%s", final.Phase, final.Message, logs)
	}
	if len(final.LocalFiles) != 1 {
		t.Fatalf("expected one local archive, got %d", len(final.LocalFiles))
	}
	for _, device := range final.Devices {
		if device.Role == "B" {
			if device.Phase != model.DeviceSkipped {
				t.Fatalf("device B should be skipped, got %s", device.Phase)
			}
		}
		if device.Role == "A" && device.Phase != model.DeviceCompleted {
			t.Fatalf("device A should complete, got %s: %s", device.Phase, device.Message)
		}
	}
	if topology.b.acceptCount() != 0 {
		t.Fatalf("device B server should not be contacted, got %d accepts", topology.b.acceptCount())
	}
	if len(topology.a.forwardedTargets()) != 0 {
		t.Fatalf("device A should not forward when B is disabled: %v", topology.a.forwardedTargets())
	}
}

func TestCollectorConcurrentCombinations(t *testing.T) {
	first := newTestTopology(t, false)
	second := newTestTopology(t, false)
	second.combo.ID = "combo-test-2"
	second.config.Combinations = []model.Combination{second.combo}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	manager := NewManager(ctx, nil)
	t.Cleanup(func() { manager.StopAll() })
	if err := manager.Start(*first.config, first.combo); err != nil {
		t.Fatalf("start first: %v", err)
	}
	if err := manager.Start(*second.config, second.combo); err != nil {
		t.Fatalf("start second: %v", err)
	}
	firstFinal := waitForTerminal(t, manager, first.combo.ID, 20*time.Second)
	secondFinal := waitForTerminal(t, manager, second.combo.ID, 20*time.Second)
	for name, state := range map[string]model.RunState{"first": firstFinal, "second": secondFinal} {
		if state.Phase != model.RunSuccess {
			t.Fatalf("%s combination failed: %s\nlogs:\n%s", name, state.Message, formatLogs(manager.Logs(state.ComboID)))
		}
		if len(state.LocalFiles) != 1 {
			t.Fatalf("%s combination should produce one archive, got %d", name, len(state.LocalFiles))
		}
	}
}

func TestCollectorKeepsDeviceASuccessWhenDeviceBFails(t *testing.T) {
	topology := newTestTopology(t, true)
	topology.combo.DeviceB.Password = "wrong-device-b-password"
	topology.config.Combinations = []model.Combination{topology.combo}
	manager, _ := startTestManager(t, *topology.config, topology.combo)
	final := waitForTerminal(t, manager, topology.combo.ID, 20*time.Second)
	logs := formatLogs(manager.Logs(topology.combo.ID))
	if final.Phase != model.RunPartial {
		t.Fatalf("expected partial, got %s: %s\nlogs:\n%s", final.Phase, final.Message, logs)
	}
	devices := map[string]model.DeviceRunState{}
	for _, device := range final.Devices {
		devices[device.Role] = device
	}
	if devices["A"].Phase != model.DeviceCompleted {
		t.Fatalf("device A should still complete, got %s: %s", devices["A"].Phase, devices["A"].Message)
	}
	if devices["B"].Phase != model.DeviceFailed {
		t.Fatalf("device B should fail, got %s", devices["B"].Phase)
	}
	if len(final.LocalFiles) != 1 {
		t.Fatalf("expected one completed archive from A, got %d", len(final.LocalFiles))
	}
}

func TestCollectorStopTerminatesRunningCollection(t *testing.T) {
	topology := newTestTopology(t, false)
	logsDir := filepath.Join(t.TempDir(), "never-ending-logs")
	topology.combo.DeviceA.CollectPaths = []string{logsDir}
	topology.combo.DeviceA.Commands = "mkdir -p " + ShellQuote(logsDir) + "\n" +
		"i=0\n" +
		"while true; do\n" +
		"  i=$((i+1))\n" +
		"  echo \"tick-$i\"\n" +
		"  sleep 0.12\n" +
		"done\n"
	topology.config.Combinations = []model.Combination{topology.combo}

	manager, _ := startTestManager(t, *topology.config, topology.combo)
	deadline := time.Now().Add(8 * time.Second)
	sawTick := false
	for time.Now().Before(deadline) {
		for _, entry := range manager.Logs(topology.combo.ID) {
			if strings.Contains(entry.Message, "tick-") {
				sawTick = true
				break
			}
		}
		if sawTick {
			break
		}
		time.Sleep(40 * time.Millisecond)
	}
	if !sawTick {
		t.Fatalf("never observed command output before stopping; logs:\n%s", formatLogs(manager.Logs(topology.combo.ID)))
	}
	if err := manager.Stop(topology.combo.ID); err != nil {
		t.Fatalf("stop: %v", err)
	}
	final := waitForTerminal(t, manager, topology.combo.ID, 12*time.Second)
	if final.Phase != model.RunStopped {
		t.Fatalf("expected stopped, got %s: %s\nlogs:\n%s", final.Phase, final.Message, formatLogs(manager.Logs(topology.combo.ID)))
	}
	if !strings.Contains(formatLogs(manager.Logs(topology.combo.ID)), "停止") {
		t.Fatal("stop was not recorded in logs")
	}
}
