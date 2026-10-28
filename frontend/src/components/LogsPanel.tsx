import { useEffect, useMemo, useRef, useState } from "react";
import type { Combination, LogEntry, RunState } from "../types";
import {
  devicePhaseMeta,
  formatDateTime,
  formatTime,
  getDevice,
  isRunActive,
  runPhaseMeta,
} from "../utils";
import { Icon } from "./Icon";
import { Spinner } from "./ui";

interface LogsPanelProps {
  open: boolean;
  combo: Combination | null;
  run?: RunState;
  logs: LogEntry[];
  loading: boolean;
  stopBusy: boolean;
  onClose: () => void;
  onClear: () => void;
  onStop: () => void;
  onOpenFolder: () => void;
}

type LogFilter = "all" | "A" | "B";

async function writeClipboard(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    try {
      const area = document.createElement("textarea");
      area.value = text;
      area.style.position = "fixed";
      area.style.opacity = "0";
      document.body.appendChild(area);
      area.select();
      const ok = document.execCommand("copy");
      document.body.removeChild(area);
      return ok;
    } catch {
      return false;
    }
  }
}

function basename(path: string): string {
  const parts = path.split(/[\\/]/);
  return parts[parts.length - 1] || path;
}

export function LogsPanel({
  open,
  combo,
  run,
  logs,
  loading,
  stopBusy,
  onClose,
  onClear,
  onStop,
  onOpenFolder,
}: LogsPanelProps) {
  const [filter, setFilter] = useState<LogFilter>("all");
  const [autoScroll, setAutoScroll] = useState(true);
  const [copied, setCopied] = useState(false);
  const terminalRef = useRef<HTMLDivElement>(null);
  const closeRef = useRef<HTMLButtonElement>(null);

  const active = isRunActive(run);
  const phase = run?.phase ?? "idle";
  const meta = runPhaseMeta[phase];
  const deviceA = getDevice(run, "A");
  const deviceB = getDevice(run, "B");

  const filteredLogs = useMemo(() => {
    if (filter === "all") return logs;
    return logs.filter((entry) => !entry.device || entry.device === filter);
  }, [logs, filter]);

  useEffect(() => {
    if (!open) return;
    setFilter("all");
    setAutoScroll(true);
    setCopied(false);
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        onClose();
      }
    };
    document.addEventListener("keydown", onKeyDown);
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    const timer = window.setTimeout(() => closeRef.current?.focus(), 50);
    return () => {
      document.removeEventListener("keydown", onKeyDown);
      document.body.style.overflow = previousOverflow;
      window.clearTimeout(timer);
    };
  }, [open, onClose]);

  useEffect(() => {
    if (!open || !autoScroll) return;
    const target = terminalRef.current;
    if (target) target.scrollTop = target.scrollHeight;
  }, [filteredLogs, autoScroll, open]);

  if (!open || !combo) return null;

  const copyLogs = async () => {
    const text = filteredLogs
      .map((entry) => {
        const tag = entry.device ? "[" + entry.device + "]" : "[系统]";
        return formatTime(entry.at) + " " + tag + " " + entry.message;
      })
      .join("\n");
    const ok = await writeClipboard(text);
    setCopied(ok);
    window.setTimeout(() => setCopied(false), 1500);
  };

  const onScroll = () => {
    const node = terminalRef.current;
    if (!node) return;
    setAutoScroll(node.scrollHeight - node.scrollTop - node.clientHeight < 32);
  };

  const bConfigured = Boolean(combo.deviceB.ip.trim());

  return (
    <div
      className="logs-overlay"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <aside className="logs-panel" role="dialog" aria-modal="true" aria-label="采集日志">
        <header className="logs-head">
          <div>
            <h2>采集日志</h2>
            <p>{combo.name} · {combo.cloud.ip}</p>
          </div>
          <div className="logs-head-actions">
            {active ? (
              <button type="button" className="btn btn-danger-outline btn-sm" onClick={onStop} disabled={stopBusy}>
                {stopBusy ? <Spinner /> : <Icon name="stop" size={13} />}
                停止采集
              </button>
            ) : null}
            <button ref={closeRef} type="button" className="icon-button" onClick={onClose} aria-label="关闭日志">
              <Icon name="x" size={19} />
            </button>
          </div>
        </header>

        <div className="logs-summary">
          <div className="logs-status-line">
            <span className={"status-badge " + meta.tone}>{meta.label}</span>
            <strong title={run?.message}>{run?.message || "尚未开始采集"}</strong>
          </div>
          <small>
            {run?.startedAt ? "开始时间 " + formatDateTime(run.startedAt) : "还没有运行记录"}
            {run?.finishedAt ? " · 结束时间 " + formatDateTime(run.finishedAt) : ""}
          </small>
          <div className="device-progress">
            <DeviceProgress role="A" state={deviceA} configured={Boolean(combo.deviceA.ip)} />
            <DeviceProgress role="B" state={deviceB} configured={bConfigured} />
          </div>
        </div>

        {run?.localFiles && run.localFiles.length > 0 ? (
          <div className="logs-files">
            <div className="logs-files-head">
              <span>已下载日志（{run.localFiles.length}）</span>
              <button type="button" className="btn btn-text btn-sm" onClick={onOpenFolder}>
                打开下载目录
              </button>
            </div>
            {run.localFiles.map((file) => (
              <div className="logs-file-row" key={file}>
                <span className="logs-file-name" title={file}>{basename(file)}</span>
                <button
                  type="button"
                  className="btn btn-text btn-sm"
                  onClick={async () => {
                    const ok = await writeClipboard(file);
                    setCopied(ok);
                    window.setTimeout(() => setCopied(false), 1500);
                  }}
                >
                  复制路径
                </button>
              </div>
            ))}
          </div>
        ) : null}

        <div className="logs-toolbar">
          <div className="segment" role="group" aria-label="筛选设备">
            <button type="button" className={filter === "all" ? "active" : ""} onClick={() => setFilter("all")}>
              全部
            </button>
            <button type="button" className={filter === "A" ? "active" : ""} onClick={() => setFilter("A")}>
              设备 A
            </button>
            <button type="button" className={filter === "B" ? "active" : ""} onClick={() => setFilter("B")}>
              设备 B
            </button>
          </div>
          <div className="logs-tools">
            <button type="button" className="btn btn-text btn-sm" onClick={() => setAutoScroll((value) => !value)}>
              自动滚动：{autoScroll ? "开" : "关"}
            </button>
            <button type="button" className="btn btn-text btn-sm" onClick={copyLogs}>
              {copied ? "已复制" : "复制"}
            </button>
            <button type="button" className="btn btn-text btn-sm" onClick={onClear}>
              清空
            </button>
          </div>
        </div>

        <div className="terminal" ref={terminalRef} onScroll={onScroll}>
          {loading ? (
            <div className="terminal-empty">
              <Spinner /> 正在加载日志…
            </div>
          ) : filteredLogs.length === 0 ? (
            <div className="terminal-empty">暂无日志</div>
          ) : (
            filteredLogs.map((entry) => (
              <div key={entry.seq} className={"log-line level-" + entry.level}>
                <span className="log-time">{formatTime(entry.at)}</span>
                <span className={"log-device device-" + (entry.device || "system")}>
                  {entry.device || "系统"}
                </span>
                <span className="log-text">{entry.message}</span>
              </div>
            ))
          )}
        </div>
        <div className="logs-footer">
          <span>共 {filteredLogs.length} 条日志</span>
          <span>{active ? "采集进行中" : "采集未运行"}</span>
        </div>
      </aside>
    </div>
  );
}

function DeviceProgress({
  role,
  state,
  configured,
}: {
  role: "A" | "B";
  state?: RunState["devices"][number];
  configured: boolean;
}) {
  if (!configured) {
    return (
      <div className="device-progress-row">
        <span className="progress-role">设备 {role}</span>
        <span className="progress-state text-muted">未接入</span>
      </div>
    );
  }
  const phase = state?.phase ?? "idle";
  const meta = devicePhaseMeta[phase];
  return (
    <div className="device-progress-row">
      <span className="progress-role">设备 {role}</span>
      <span className={"progress-state " + meta.tone}>{meta.label}</span>
      <span className="progress-message" title={state?.message}>{state?.message || "等待开始"}</span>
    </div>
  );
}
