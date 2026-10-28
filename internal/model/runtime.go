package model

const (
	RunIdle    = "idle"
	RunRunning = "running"
	RunSuccess = "success"
	RunPartial = "partial"
	RunFailed  = "failed"
	RunStopped = "stopped"

	DeviceIdle        = "idle"
	DeviceSkipped     = "skipped"
	DeviceConnecting  = "connecting"
	DeviceCollecting  = "collecting"
	DeviceArchiving   = "archiving"
	DeviceUploading   = "uploading"
	DeviceDownloading = "downloading"
	DeviceCompleted   = "completed"
	DeviceFailed      = "failed"
	DeviceStopped     = "stopped"
)

const (
	LogInfo    = "info"
	LogSuccess = "success"
	LogWarn    = "warn"
	LogError   = "error"
	LogOutput  = "output"
	LogCommand = "command"
)

type DeviceRunState struct {
	Role        string `json:"role"`
	IP          string `json:"ip"`
	Phase       string `json:"phase"`
	Message     string `json:"message"`
	ArchiveName string `json:"archiveName"`
	CloudPath   string `json:"cloudPath"`
	LocalPath   string `json:"localPath"`
	Bytes       int64  `json:"bytes"`
	StartedAt   string `json:"startedAt"`
	FinishedAt  string `json:"finishedAt"`
}

type RunState struct {
	ComboID    string           `json:"comboId"`
	ComboName  string           `json:"comboName"`
	Phase      string           `json:"phase"`
	Message    string           `json:"message"`
	Devices    []DeviceRunState `json:"devices"`
	LocalFiles []string         `json:"localFiles"`
	StartedAt  string           `json:"startedAt"`
	FinishedAt string           `json:"finishedAt"`
}

type LogEntry struct {
	Seq     int64  `json:"seq"`
	ComboID string `json:"comboId"`
	Device  string `json:"device"`
	Level   string `json:"level"`
	Message string `json:"message"`
	At      string `json:"at"`
}

type AppState struct {
	Config      AppConfig  `json:"config"`
	ConfigPath  string     `json:"configPath"`
	ConfigError string     `json:"configError"`
	Runs        []RunState `json:"runs"`
}

type Event struct {
	Kind string    `json:"kind"`
	Run  *RunState `json:"run,omitempty"`
	Log  *LogEntry `json:"log,omitempty"`
}
