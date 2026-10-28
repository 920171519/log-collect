import type {
  Combination,
  DeviceConfig,
  DevicePhase,
  DeviceRunState,
  RunPhase,
  RunState,
} from "./types";

export function generateID(): string {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return "combo-" + crypto.randomUUID().replaceAll("-", "").slice(0, 16);
  }
  return "combo-" + Math.random().toString(16).slice(2) + Date.now().toString(16);
}

export function hasValue(value: string | undefined | null): boolean {
  return typeof value === "string" && value.trim().length > 0;
}

export function isDeviceComplete(device: DeviceConfig): boolean {
  return (
    hasValue(device.ip) &&
    Number(device.port) > 0 &&
    Number(device.port) <= 65535 &&
    hasValue(device.username) &&
    hasValue(device.password) &&
    hasValue(device.commands) &&
    hasValue(device.endMarker) &&
    !device.endMarker.includes("\n") &&
    !device.endMarker.includes("\r") &&
    Array.isArray(device.collectPaths) &&
    device.collectPaths.some((p) => hasValue(p))
  );
}

export function isCombinationRunnable(combo: Combination): boolean {
  const cloudReady =
    hasValue(combo.cloud.ip) &&
    Number(combo.cloud.port) > 0 &&
    Number(combo.cloud.port) <= 65535 &&
    hasValue(combo.cloud.username) &&
    hasValue(combo.cloud.password);
  const aReady = isDeviceComplete(combo.deviceA);
  const bReady = !hasValue(combo.deviceB.ip) || isDeviceComplete(combo.deviceB);
  return cloudReady && aReady && bReady;
}

export function missingCombinationReason(combo: Combination): string {
  if (!hasValue(combo.cloud.ip)) return "请填写云主机 IP";
  if (!hasValue(combo.cloud.username)) return "请填写云主机用户名";
  if (!hasValue(combo.cloud.password)) return "请填写云主机密码";
  if (!hasValue(combo.deviceA.ip)) return "请填写设备 A 的 IP";
  if (!hasValue(combo.deviceA.username)) return "请填写设备 A 的用户名";
  if (!hasValue(combo.deviceA.password)) return "请填写设备 A 的密码";
  if (!hasValue(combo.deviceA.commands)) return "请填写设备 A 的采集命令";
  if (!hasValue(combo.deviceA.endMarker)) return "请填写设备 A 的结束标记";
  if (!combo.deviceA.collectPaths.some((p) => hasValue(p))) return "请填写设备 A 的日志路径";
  if (hasValue(combo.deviceB.ip) && !isDeviceComplete(combo.deviceB)) {
    return "设备 B 已填写 IP，请补全 B 的登录信息和采集配置";
  }
  return "";
}

export function splitPaths(text: string): string[] {
  return text
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);
}

export function joinPaths(paths: string[]): string {
  return (paths ?? []).join("\n");
}

export function formatDateTime(value: string): string {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  const pad = (n: number) => String(n).padStart(2, "0");
  const month = date.getMonth() + 1;
  const day = date.getDate();
  return month + "月" + day + "日 " + pad(date.getHours()) + ":" + pad(date.getMinutes()) + ":" + pad(date.getSeconds());
}

export function formatTime(value: string): string {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  const pad = (n: number) => String(n).padStart(2, "0");
  return pad(date.getHours()) + ":" + pad(date.getMinutes()) + ":" + pad(date.getSeconds());
}

export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let value = bytes;
  let index = 0;
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024;
    index += 1;
  }
  return String(value >= 10 || index === 0 ? Math.round(value) : value.toFixed(1)) + " " + units[index];
}

export interface PhaseMeta {
  label: string;
  tone: "neutral" | "active" | "success" | "warning" | "danger";
}

export const runPhaseMeta: Record<RunPhase, PhaseMeta> = {
  idle: { label: "待采集", tone: "neutral" },
  running: { label: "采集中", tone: "active" },
  success: { label: "已完成", tone: "success" },
  partial: { label: "部分完成", tone: "warning" },
  failed: { label: "失败", tone: "danger" },
  stopped: { label: "已停止", tone: "warning" },
};

export const devicePhaseMeta: Record<DevicePhase, PhaseMeta> = {
  idle: { label: "等待", tone: "neutral" },
  skipped: { label: "未接入", tone: "neutral" },
  connecting: { label: "连接中", tone: "active" },
  collecting: { label: "采集命令", tone: "active" },
  archiving: { label: "打包中", tone: "active" },
  uploading: { label: "上传云主机", tone: "active" },
  downloading: { label: "下载到本地", tone: "active" },
  completed: { label: "已完成", tone: "success" },
  failed: { label: "失败", tone: "danger" },
  stopped: { label: "已停止", tone: "warning" },
};

export function isRunActive(run: RunState | undefined): boolean {
  return run?.phase === "running";
}

export function isDeviceActive(device: DeviceRunState | undefined): boolean {
  if (!device) return false;
  return ["connecting", "collecting", "archiving", "uploading", "downloading"].includes(device.phase);
}

export function getDevice(run: RunState | undefined, role: "A" | "B"): DeviceRunState | undefined {
  return run?.devices.find((device) => device.role === role);
}

export function errorMessage(error: unknown): string {
  if (error instanceof Error) return error.message;
  if (typeof error === "string") return error;
  if (error && typeof error === "object" && "message" in error) {
    return String((error as { message: unknown }).message);
  }
  return "发生未知错误";
}

export function comboFolderHint(downloadDir: string, name: string): string {
  const base = downloadDir.replace(/[\\/]+$/, "");
  return base + "/" + (name || "组合") + "-xxxx/";
}
