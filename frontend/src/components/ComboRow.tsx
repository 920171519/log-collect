import type { Combination, HostConfig, RunState } from "../types";
import {
  formatBytes,
  getDevice,
  isCombinationRunnable,
  isRunActive,
  missingCombinationReason,
  runPhaseMeta,
} from "../utils";
import { Icon } from "./Icon";
import { Spinner } from "./ui";

interface ComboRowProps {
  combo: Combination;
  run?: RunState;
  busyAction?: "start" | "stop" | null;
  disabled?: boolean;
  onStart: () => void;
  onStop: () => void;
  onOpenLogs: () => void;
  onEdit: () => void;
  onDelete: () => void;
}

function endpoint(host: HostConfig): string {
  if (!host.ip) return "未填写";
  return host.ip + ":" + host.port;
}

function endpointTitle(host: HostConfig): string {
  if (!host.ip) return "未填写";
  const user = host.username ? host.username + "@" : "";
  return user + host.ip + ":" + host.port;
}

function statusText(combo: Combination, run: RunState | undefined, runnable: boolean): string {
  if (!runnable) return missingCombinationReason(combo) || "配置不完整";
  if (!run) return "等待采集";
  if (isRunActive(run)) return run.message || "正在采集";
  if (run.phase === "success") {
    const count = run.localFiles?.length ?? 0;
    return "已下载 " + count + " 个日志归档";
  }
  if (run.phase === "failed") return "采集失败，点击「日志」查看原因";
  if (run.phase === "partial") return "部分设备未完成，点击「日志」查看详情";
  if (run.phase === "stopped") return "采集已停止";
  return run.message || "等待采集";
}

export function ComboRow({
  combo,
  run,
  busyAction,
  disabled = false,
  onStart,
  onStop,
  onOpenLogs,
  onEdit,
  onDelete,
}: ComboRowProps) {
  const runnable = isCombinationRunnable(combo);
  const active = isRunActive(run);
  const phase = run?.phase ?? "idle";
  const meta = runPhaseMeta[phase];
  const bEnabled = Boolean(combo.deviceB.ip.trim());
  const message = statusText(combo, run, runnable);
  const localFiles = run?.localFiles ?? [];
  const totalBytes = (getDevice(run, "A")?.bytes ?? 0) + (getDevice(run, "B")?.bytes ?? 0);

  return (
    <div className={"host-row phase-" + meta.tone + (active ? " is-running" : "") + (!runnable ? " is-incomplete" : "")}>
      <div className="cell name-cell">
        <div className="host-name-line">
          <span className={"row-dot " + meta.tone} />
          <span className="host-name" title={combo.name}>{combo.name || "未命名组合"}</span>
        </div>
        {!runnable ? <span className="row-tag">待完善</span> : null}
      </div>

      <div className="cell endpoint-cell">
        <code title={endpointTitle(combo.cloud)}>{endpoint(combo.cloud)}</code>
      </div>

      <div className="cell endpoint-cell">
        <code title={endpointTitle(combo.deviceA)}>{endpoint(combo.deviceA)}</code>
      </div>

      <div className="cell endpoint-cell">
        {bEnabled ? (
          <code title={endpointTitle(combo.deviceB)}>{endpoint(combo.deviceB)}</code>
        ) : (
          <span className="text-muted">未接入</span>
        )}
      </div>

      <div className="cell status-cell">
        <div className="status-line">
          <span className={"status-badge " + meta.tone}>{meta.label}</span>
          {!active && localFiles.length > 0 ? (
            <span className="row-size">{formatBytes(totalBytes)}</span>
          ) : null}
        </div>
        <span className="status-message" title={message}>{message}</span>
      </div>

      <div className="cell actions-cell">
        {active ? (
          <button
            type="button"
            className="btn btn-danger-outline btn-sm"
            onClick={onStop}
            disabled={disabled || busyAction !== null}
          >
            {busyAction === "stop" ? <Spinner /> : <Icon name="stop" size={13} />}
            停止
          </button>
        ) : (
          <button
            type="button"
            className="btn btn-primary btn-sm"
            onClick={onStart}
            disabled={disabled || !runnable || busyAction !== null}
            title={!runnable ? missingCombinationReason(combo) : "开始采集"}
          >
            {busyAction === "start" ? <Spinner /> : <Icon name="play" size={13} />}
            采集
          </button>
        )}
        <button type="button" className="btn btn-text btn-sm" onClick={onOpenLogs}>
          日志
        </button>
        <button
          type="button"
          className="btn btn-text btn-sm"
          onClick={onEdit}
          disabled={disabled || active}
          title={active ? "采集中不可编辑" : "编辑"}
        >
          编辑
        </button>
        <button
          type="button"
          className="icon-button row-delete"
          onClick={onDelete}
          disabled={disabled || active}
          title={active ? "采集中不可删除" : "删除"}
          aria-label="删除组合"
        >
          <Icon name="trash" size={15} />
        </button>
      </div>
    </div>
  );
}
