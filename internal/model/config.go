package model

import (
	"fmt"
	"net"
	"strings"
)

const (
	ConfigVersion = 1

	DefaultCloudPort       = 22
	DefaultDevicePort      = 22
	DefaultEndMarker       = "__LOGCOLLECT_DONE__"
	DefaultArchiveTemplate = "{combo}_{device}_{timestamp}.tar.gz"
	DefaultCloudStageDir   = "/tmp/logcollector-staging"
	DefaultDeviceTempDir   = "/tmp/logcollector-transfer"
	DefaultTimeoutMinutes  = 30
)

// HostConfig is the SSH endpoint and credential set for a cloud host.
type HostConfig struct {
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// DeviceConfig is the SSH endpoint, credential set and collection recipe for a device.
// Device B uses the SSH connection to Device A as a jump host, mirroring
// "ssh device-b" typed in Device A's terminal.
type DeviceConfig struct {
	IP           string   `json:"ip"`
	Port         int      `json:"port"`
	Username     string   `json:"username"`
	Password     string   `json:"password"`
	Commands     string   `json:"commands"`
	CollectPaths []string `json:"collectPaths"`
	EndMarker    string   `json:"endMarker"`
}

type Combination struct {
	ID      string       `json:"id"`
	Name    string       `json:"name"`
	Cloud   HostConfig   `json:"cloud"`
	DeviceA DeviceConfig `json:"deviceA"`
	DeviceB DeviceConfig `json:"deviceB"`
}

type AppConfig struct {
	Version             int           `json:"version"`
	DownloadDir         string        `json:"downloadDir"`
	ArchiveNameTemplate string        `json:"archiveNameTemplate"`
	CloudStageDir       string        `json:"cloudStageDir"`
	DeviceTempDir       string        `json:"deviceTempDir"`
	KeepCloudCopy       bool          `json:"keepCloudCopy"`
	TimeoutMinutes      int           `json:"timeoutMinutes"`
	Combinations        []Combination `json:"combinations"`
}

func DefaultDevice(role string) DeviceConfig {
	commands := fmt.Sprintf(`# 示例采集命令，请替换为设备 %s 的实际日志采集命令
# 支持多行 POSIX shell 命令；结束标记由程序自动追加
LOG_DIR=/tmp/logcollector-logs
rm -rf "$LOG_DIR"
mkdir -p "$LOG_DIR"
hostname > "$LOG_DIR/hostname.txt"
date > "$LOG_DIR/date.txt"
uname -a > "$LOG_DIR/system.txt" 2>&1 || true
df -h > "$LOG_DIR/disk.txt" 2>&1 || true
`, role)
	return DeviceConfig{
		Port:         DefaultDevicePort,
		Username:     "root",
		Commands:     commands,
		CollectPaths: []string{"/tmp/logcollector-logs"},
		EndMarker:    DefaultEndMarker,
	}
}

func DefaultConfig() AppConfig {
	return AppConfig{
		Version:             ConfigVersion,
		DownloadDir:         "",
		ArchiveNameTemplate: DefaultArchiveTemplate,
		CloudStageDir:       DefaultCloudStageDir,
		DeviceTempDir:       DefaultDeviceTempDir,
		KeepCloudCopy:       false,
		TimeoutMinutes:      DefaultTimeoutMinutes,
		Combinations:        []Combination{},
	}
}

func NewCombination(name string) Combination {
	return Combination{
		Name:    name,
		Cloud:   HostConfig{Port: DefaultCloudPort},
		DeviceA: DefaultDevice("A"),
		DeviceB: DefaultDevice("B"),
	}
}

// Normalize applies safe defaults and trims values without rejecting them.
func (c *AppConfig) Normalize() {
	if c.Version <= 0 {
		c.Version = ConfigVersion
	}
	c.DownloadDir = strings.TrimSpace(c.DownloadDir)
	c.ArchiveNameTemplate = strings.TrimSpace(c.ArchiveNameTemplate)
	if c.ArchiveNameTemplate == "" {
		c.ArchiveNameTemplate = DefaultArchiveTemplate
	}
	c.CloudStageDir = strings.TrimSpace(c.CloudStageDir)
	if c.CloudStageDir == "" {
		c.CloudStageDir = DefaultCloudStageDir
	}
	c.DeviceTempDir = strings.TrimSpace(c.DeviceTempDir)
	if c.DeviceTempDir == "" {
		c.DeviceTempDir = DefaultDeviceTempDir
	}
	if c.TimeoutMinutes <= 0 {
		c.TimeoutMinutes = DefaultTimeoutMinutes
	}
	if c.Combinations == nil {
		c.Combinations = []Combination{}
	}
	for i := range c.Combinations {
		normalizeCombination(&c.Combinations[i])
	}
}

func normalizeCombination(c *Combination) {
	c.ID = strings.TrimSpace(c.ID)
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		if strings.TrimSpace(c.Cloud.IP) != "" {
			c.Name = strings.TrimSpace(c.Cloud.IP)
		} else {
			c.Name = "未命名组合"
		}
	}
	normalizeHost(&c.Cloud)
	normalizeDevice(&c.DeviceA)
	normalizeDevice(&c.DeviceB)
	// An empty device B IP means that this cloud host has no B device. Keep
	// the remaining B fields so an operator can disable B temporarily without
	// losing its saved credentials and command recipe.
}

func normalizeHost(h *HostConfig) {
	h.IP = strings.TrimSpace(h.IP)
	h.Username = strings.TrimSpace(h.Username)
	if h.Port <= 0 || h.Port > 65535 {
		h.Port = DefaultCloudPort
	}
}

func normalizeDevice(d *DeviceConfig) {
	d.IP = strings.TrimSpace(d.IP)
	d.Username = strings.TrimSpace(d.Username)
	d.EndMarker = strings.TrimSpace(d.EndMarker)
	if d.Port <= 0 || d.Port > 65535 {
		d.Port = DefaultDevicePort
	}
	if d.EndMarker == "" {
		d.EndMarker = DefaultEndMarker
	}
	paths := make([]string, 0, len(d.CollectPaths))
	for _, p := range d.CollectPaths {
		p = strings.TrimSpace(p)
		if p != "" {
			paths = append(paths, p)
		}
	}
	d.CollectPaths = paths
}

func validateHost(label string, h HostConfig) error {
	if strings.TrimSpace(h.IP) == "" {
		return fmt.Errorf("%s IP 不能为空", label)
	}
	if strings.ContainsAny(h.IP, " 	\r\n") {
		return fmt.Errorf("%s IP 格式不正确", label)
	}
	if h.Port <= 0 || h.Port > 65535 {
		return fmt.Errorf("%s SSH 端口应在 1-65535 之间", label)
	}
	if strings.TrimSpace(h.Username) == "" {
		return fmt.Errorf("%s 用户名不能为空", label)
	}
	if h.Password == "" {
		return fmt.Errorf("%s 密码不能为空", label)
	}
	return nil
}

func validateDevice(label string, d DeviceConfig) error {
	if strings.TrimSpace(d.IP) == "" {
		return fmt.Errorf("%s IP 不能为空", label)
	}
	if strings.ContainsAny(d.IP, " 	\r\n") {
		return fmt.Errorf("%s IP 格式不正确", label)
	}
	if d.Port <= 0 || d.Port > 65535 {
		return fmt.Errorf("%s SSH 端口应在 1-65535 之间", label)
	}
	if strings.TrimSpace(d.Username) == "" {
		return fmt.Errorf("%s 用户名不能为空", label)
	}
	if d.Password == "" {
		return fmt.Errorf("%s 密码不能为空", label)
	}
	if strings.TrimSpace(d.Commands) == "" {
		return fmt.Errorf("%s 采集命令不能为空", label)
	}
	if strings.TrimSpace(d.EndMarker) == "" {
		return fmt.Errorf("%s 结束标记不能为空", label)
	}
	if strings.ContainsAny(d.EndMarker, "\r\n") {
		return fmt.Errorf("%s 结束标记必须是一行文本", label)
	}
	if len(d.CollectPaths) == 0 {
		return fmt.Errorf("%s 日志路径不能为空", label)
	}
	for _, p := range d.CollectPaths {
		if strings.TrimSpace(p) == "" || strings.ContainsRune(p, 0) {
			return fmt.Errorf("%s 日志路径不能包含空路径", label)
		}
	}
	return nil
}

// Validate checks global settings. Individual combinations may be incomplete
// drafts and are checked by Combination.Validate before a run starts.
func (c AppConfig) Validate() error {
	if strings.TrimSpace(c.ArchiveNameTemplate) == "" {
		return fmt.Errorf("归档文件名模板不能为空")
	}
	if !strings.Contains(c.ArchiveNameTemplate, "{device}") {
		return fmt.Errorf("归档文件名模板必须包含 {device}")
	}
	if c.TimeoutMinutes < 1 || c.TimeoutMinutes > 24*60 {
		return fmt.Errorf("超时时间应在 1-1440 分钟之间")
	}
	if c.CloudStageDir == "" || !strings.HasPrefix(c.CloudStageDir, "/") {
		return fmt.Errorf("云主机暂存目录必须是绝对路径")
	}
	if c.DeviceTempDir == "" || !strings.HasPrefix(c.DeviceTempDir, "/") {
		return fmt.Errorf("设备临时目录必须是绝对路径")
	}
	seen := make(map[string]struct{}, len(c.Combinations))
	for i, combo := range c.Combinations {
		label := combo.Name
		if label == "" {
			label = fmt.Sprintf("第 %d 个组合", i+1)
		}
		if combo.ID != "" {
			if _, ok := seen[combo.ID]; ok {
				return fmt.Errorf("组合 %s 的标识重复", label)
			}
			seen[combo.ID] = struct{}{}
		}
	}
	return nil
}

func (c Combination) Validate() error {
	label := c.Name
	if label == "" {
		label = "该组合"
	}
	if err := validateHost(fmt.Sprintf("%s 的云主机", label), c.Cloud); err != nil {
		return err
	}
	if err := validateDevice(fmt.Sprintf("%s 的设备 A", label), c.DeviceA); err != nil {
		return err
	}
	// An empty Device B IP means the cloud host has no B device.
	if strings.TrimSpace(c.DeviceB.IP) != "" {
		if err := validateDevice(fmt.Sprintf("%s 的设备 B", label), c.DeviceB); err != nil {
			return err
		}
	}
	return nil
}

func (c Combination) IsRunnable() bool {
	return c.Validate() == nil
}

func (c AppConfig) ActiveCombinationCount() int {
	count := 0
	for _, combo := range c.Combinations {
		if strings.TrimSpace(combo.Cloud.IP) != "" && strings.TrimSpace(combo.DeviceA.IP) != "" {
			count++
		}
	}
	return count
}

func JoinHostPort(host string, port int) string {
	return net.JoinHostPort(strings.TrimSpace(host), fmt.Sprintf("%d", port))
}

func Clone(c AppConfig) AppConfig {
	out := c
	out.Combinations = make([]Combination, len(c.Combinations))
	for i, combo := range c.Combinations {
		out.Combinations[i] = combo
		out.Combinations[i].DeviceA.CollectPaths = append([]string(nil), combo.DeviceA.CollectPaths...)
		out.Combinations[i].DeviceB.CollectPaths = append([]string(nil), combo.DeviceB.CollectPaths...)
	}
	return out
}
