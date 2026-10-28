export interface HostConfig {
  ip: string;
  port: number;
  username: string;
  password: string;
}

export interface DeviceConfig {
  ip: string;
  port: number;
  username: string;
  password: string;
  commands: string;
  collectPaths: string[];
  endMarker: string;
}

export interface Combination {
  id: string;
  name: string;
  cloud: HostConfig;
  deviceA: DeviceConfig;
  deviceB: DeviceConfig;
}

export interface AppConfig {
  version: number;
  downloadDir: string;
  archiveNameTemplate: string;
  cloudStageDir: string;
  deviceTempDir: string;
  keepCloudCopy: boolean;
  timeoutMinutes: number;
  combinations: Combination[];
}

export type RunPhase = "idle" | "running" | "success" | "partial" | "failed" | "stopped";
export type DevicePhase =
  | "idle"
  | "skipped"
  | "connecting"
  | "collecting"
  | "archiving"
  | "uploading"
  | "downloading"
  | "completed"
  | "failed"
  | "stopped";

export interface DeviceRunState {
  role: "A" | "B" | string;
  ip: string;
  phase: DevicePhase;
  message: string;
  archiveName: string;
  cloudPath: string;
  localPath: string;
  bytes: number;
  startedAt: string;
  finishedAt: string;
}

export interface RunState {
  comboId: string;
  comboName: string;
  phase: RunPhase;
  message: string;
  devices: DeviceRunState[];
  localFiles: string[];
  startedAt: string;
  finishedAt: string;
}

export interface AppState {
  config: AppConfig;
  configPath: string;
  configError: string;
  runs: RunState[];
}

export interface LogEntry {
  seq: number;
  comboId: string;
  device: string;
  level: "info" | "success" | "warn" | "error" | "output" | "command" | string;
  message: string;
  at: string;
}

export interface CollectorEvent {
  kind: "run" | "log";
  run?: RunState;
  log?: LogEntry;
}
