import { useCallback, useEffect, useRef, useState } from "react";
import { EventsOn } from "../wailsjs/runtime/runtime";
import {
  chooseDownloadDir,
  clearLogs as clearLogsApi,
  getAppState,
  getLogs,
  newCombinationDraft,
  openConfigFolder,
  openDownloadFolder,
  saveConfig,
  startAll as startAllApi,
  startCombination,
  stopAll as stopAllApi,
  stopCombination,
} from "./api";
import type { AppConfig, Combination, CollectorEvent, LogEntry, RunState } from "./types";
import { ComboEditor } from "./components/ComboEditor";
import { ComboRow } from "./components/ComboRow";
import { Icon } from "./components/Icon";
import { LogsPanel } from "./components/LogsPanel";
import { SettingsModal } from "./components/SettingsModal";
import {
  ConfirmDialog,
  Spinner,
  ToastStack,
  type ToastItem,
  type ToastKind,
} from "./components/ui";
import { errorMessage, generateID, isCombinationRunnable, isRunActive } from "./utils";

function mergeLogs(left: LogEntry[], right: LogEntry[]): LogEntry[] {
  const map = new Map<number, LogEntry>();
  for (const entry of left) map.set(entry.seq, entry);
  for (const entry of right) map.set(entry.seq, entry);
  return Array.from(map.values()).sort((a, b) => a.seq - b.seq).slice(-3000);
}

function idleRunState(combo: Combination): RunState {
  return {
    comboId: combo.id,
    comboName: combo.name,
    phase: "idle",
    message: "尚未开始",
    devices: [
      {
        role: "A",
        ip: combo.deviceA.ip,
        phase: "idle",
        message: "等待采集",
        archiveName: "",
        cloudPath: "",
        localPath: "",
        bytes: 0,
        startedAt: "",
        finishedAt: "",
      },
      {
        role: "B",
        ip: combo.deviceB.ip,
        phase: combo.deviceB.ip.trim() ? "idle" : "skipped",
        message: combo.deviceB.ip.trim() ? "等待采集" : "未配置设备 B",
        archiveName: "",
        cloudPath: "",
        localPath: "",
        bytes: 0,
        startedAt: "",
        finishedAt: "",
      },
    ],
    localFiles: [],
    startedAt: "",
    finishedAt: "",
  };
}

function App() {
  const [config, setConfig] = useState<AppConfig | null>(null);
  const [runs, setRuns] = useState<Record<string, RunState>>({});
  const [configPath, setConfigPath] = useState("");
  const [configError, setConfigError] = useState("");
  const [booting, setBooting] = useState(true);
  const [bootError, setBootError] = useState("");
  const [adding, setAdding] = useState(false);
  const [savingConfig, setSavingConfig] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [editor, setEditor] = useState<{ mode: "create" | "edit"; initial: Combination } | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Combination | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [activeLogsId, setActiveLogsId] = useState<string | null>(null);
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [logsLoading, setLogsLoading] = useState(false);
  const [comboBusy, setComboBusy] = useState<Record<string, "start" | "stop">>({});
  const [bulkBusy, setBulkBusy] = useState<"start" | "stop" | null>(null);
  const [toasts, setToasts] = useState<ToastItem[]>([]);

  const activeLogsRef = useRef<string | null>(null);
  const logsRequestRef = useRef(0);
  const toastCounter = useRef(0);
  const mountedRef = useRef(true);

  const showToast = useCallback((kind: ToastKind, title: string, message?: string) => {
    toastCounter.current += 1;
    const id = toastCounter.current;
    setToasts((current) => [...current.slice(-2), { id, kind, title, message }]);
    window.setTimeout(() => {
      setToasts((current) => current.filter((toast) => toast.id !== id));
    }, kind === "error" ? 7000 : 4200);
  }, []);

  const applyAppState = useCallback((state: Awaited<ReturnType<typeof getAppState>>) => {
    setConfig(state.config);
    setConfigPath(state.configPath);
    setConfigError(state.configError);
    const nextRuns: Record<string, RunState> = {};
    for (const run of state.runs) nextRuns[run.comboId] = run;
    setRuns(nextRuns);
  }, []);

  const loadState = useCallback(async (reportError: boolean) => {
    try {
      const state = await getAppState();
      if (!mountedRef.current) return;
      applyAppState(state);
      setBootError("");
    } catch (loadError) {
      if (!mountedRef.current) return;
      const message = errorMessage(loadError);
      if (reportError) setBootError(message);
      else showToast("error", "读取状态失败", message);
    }
  }, [applyAppState, showToast]);

  useEffect(() => {
    mountedRef.current = true;
    setBooting(true);
    void loadState(true).finally(() => {
      if (mountedRef.current) setBooting(false);
    });
    return () => {
      mountedRef.current = false;
    };
  }, [loadState]);

  useEffect(() => {
    const dispose = EventsOn("collector:event", (payload: CollectorEvent) => {
      if (!payload || !payload.kind) return;
      if (payload.kind === "run" && payload.run) {
        const run = payload.run;
        setRuns((current) => ({ ...current, [run.comboId]: run }));
      }
      if (payload.kind === "log" && payload.log) {
        const entry = payload.log;
        if (activeLogsRef.current !== entry.comboId) return;
        setLogs((current) => {
          if (current.some((item) => item.seq === entry.seq)) return current;
          return [...current, entry].slice(-3000);
        });
      }
    });
    return () => {
      if (typeof dispose === "function") dispose();
    };
  }, []);

  const persistConfig = useCallback(async (next: AppConfig): Promise<AppConfig> => {
    setSavingConfig(true);
    try {
      const saved = await saveConfig(next);
      if (mountedRef.current) {
        setConfig(saved);
        setConfigError("");
        setRuns((current) => {
          const merged = { ...current };
          for (const combo of saved.combinations) {
            if (!merged[combo.id]) merged[combo.id] = idleRunState(combo);
          }
          return merged;
        });
      }
      try {
        const state = await getAppState();
        if (mountedRef.current) applyAppState(state);
      } catch {
        // Config was saved; state can be refreshed later.
      }
      return saved;
    } finally {
      if (mountedRef.current) setSavingConfig(false);
    }
  }, [applyAppState]);

  const openLogs = useCallback(async (comboId: string) => {
    activeLogsRef.current = comboId;
    setActiveLogsId(comboId);
    setLogs([]);
    setLogsLoading(true);
    const requestId = ++logsRequestRef.current;
    try {
      const fetched = await getLogs(comboId);
      if (logsRequestRef.current !== requestId || activeLogsRef.current !== comboId) return;
      setLogs((current) => mergeLogs(current, fetched));
    } catch (logError) {
      if (logsRequestRef.current === requestId) {
        showToast("error", "加载日志失败", errorMessage(logError));
      }
    } finally {
      if (logsRequestRef.current === requestId) setLogsLoading(false);
    }
  }, [showToast]);

  const closeLogs = useCallback(() => {
    activeLogsRef.current = null;
    logsRequestRef.current += 1;
    setActiveLogsId(null);
    setLogsLoading(false);
  }, []);

  const handleAdd = async () => {
    if (adding || editor) return;
    setAdding(true);
    try {
      const draft = await newCombinationDraft();
      draft.id = generateID();
      setEditor({ mode: "create", initial: draft });
    } catch (addError) {
      showToast("error", "无法新建云主机", errorMessage(addError));
    } finally {
      setAdding(false);
    }
  };

  const handleEdit = (combo: Combination) => {
    if (isRunActive(runs[combo.id])) {
      showToast("warning", "正在采集", "请先停止该任务，再编辑配置。");
      return;
    }
    setEditor({ mode: "edit", initial: JSON.parse(JSON.stringify(combo)) as Combination });
  };

  const handleSaveCombo = async (combo: Combination) => {
    if (!config) throw new Error("应用尚未初始化");
    const mode = editor?.mode;
    let combinations: Combination[];
    if (mode === "edit") {
      combinations = config.combinations.map((item) => (item.id === combo.id ? combo : item));
    } else {
      combinations = [...config.combinations, combo];
    }
    await persistConfig({ ...config, combinations });
    setEditor(null);
    window.scrollTo({ top: 0, behavior: "smooth" });
    showToast("success", mode === "edit" ? "已保存修改" : "已添加云主机");
  };

  const handleSaveSettings = async (next: AppConfig) => {
    await persistConfig(next);
    setSettingsOpen(false);
    showToast("success", "设置已保存");
  };

  const handleManualSave = async () => {
    if (!config || savingConfig) return;
    try {
      await persistConfig(config);
      showToast("success", "配置已保存");
    } catch (saveError) {
      showToast("error", "保存失败", errorMessage(saveError));
    }
  };

  const handleChooseDownloadDir = async () => {
    if (!config) return;
    try {
      const selected = await chooseDownloadDir(config.downloadDir);
      if (!selected) return;
      await persistConfig({ ...config, downloadDir: selected });
      showToast("success", "下载目录已更新");
    } catch (dirError) {
      showToast("error", "修改下载目录失败", errorMessage(dirError));
    }
  };

  const handleOpenDownloadFolder = async () => {
    try {
      await openDownloadFolder();
    } catch (openError) {
      showToast("error", "无法打开下载目录", errorMessage(openError));
    }
  };

  const handleOpenConfigFolder = async () => {
    try {
      await openConfigFolder();
    } catch (openError) {
      showToast("error", "无法打开配置目录", errorMessage(openError));
    }
  };

  const handleStart = async (comboId: string) => {
    setComboBusy((current) => ({ ...current, [comboId]: "start" }));
    try {
      await startCombination(comboId);
      await loadState(false);
    } catch (startError) {
      showToast("error", "启动采集失败", errorMessage(startError));
    } finally {
      setComboBusy((current) => {
        const next = { ...current };
        delete next[comboId];
        return next;
      });
    }
  };

  const handleStop = async (comboId: string) => {
    setComboBusy((current) => ({ ...current, [comboId]: "stop" }));
    try {
      await stopCombination(comboId);
      showToast("info", "已发送停止请求");
      await loadState(false);
    } catch (stopError) {
      showToast("warning", "停止失败", errorMessage(stopError));
    } finally {
      setComboBusy((current) => {
        const next = { ...current };
        delete next[comboId];
        return next;
      });
    }
  };

  const handleStartAll = async () => {
    if (bulkBusy) return;
    setBulkBusy("start");
    try {
      const count = await startAllApi();
      showToast("success", "已开始批量采集", "共 " + count + " 个云主机");
      await loadState(false);
    } catch (startError) {
      showToast("warning", "批量采集提示", errorMessage(startError));
      await loadState(false);
    } finally {
      setBulkBusy(null);
    }
  };

  const handleStopAll = async () => {
    if (bulkBusy) return;
    setBulkBusy("stop");
    try {
      const count = await stopAllApi();
      showToast("info", "已发送停止请求", "共 " + count + " 个任务");
      await loadState(false);
    } catch (stopError) {
      showToast("warning", "批量停止", errorMessage(stopError));
    } finally {
      setBulkBusy(null);
    }
  };

  const handleDelete = async () => {
    if (!config || !deleteTarget || deleting) return;
    if (isRunActive(runs[deleteTarget.id])) {
      showToast("warning", "正在采集", "请先停止该任务。");
      return;
    }
    setDeleting(true);
    try {
      await persistConfig({
        ...config,
        combinations: config.combinations.filter((item) => item.id !== deleteTarget.id),
      });
      showToast("success", "已删除云主机", deleteTarget.name);
      setDeleteTarget(null);
    } catch (deleteError) {
      showToast("error", "删除失败", errorMessage(deleteError));
    } finally {
      setDeleting(false);
    }
  };

  const handleClearLogs = async () => {
    if (!activeLogsId) return;
    const comboId = activeLogsId;
    try {
      await clearLogsApi(comboId);
      if (activeLogsRef.current === comboId) setLogs([]);
    } catch (clearError) {
      showToast("error", "清空日志失败", errorMessage(clearError));
    }
  };

  const copyConfigPath = async () => {
    if (!configPath) return;
    try {
      await navigator.clipboard.writeText(configPath);
      showToast("success", "已复制配置路径");
    } catch {
      showToast("error", "复制失败");
    }
  };

  const combos = config?.combinations ?? [];
  const runningCount = combos.filter((combo) => isRunActive(runs[combo.id])).length;
  const runnableCount = combos.filter(isCombinationRunnable).length;
  const incompleteCount = combos.length - runnableCount;
  const activeLogsCombo = activeLogsId ? combos.find((combo) => combo.id === activeLogsId) ?? null : null;
  const activeLogsRun = activeLogsId ? runs[activeLogsId] : undefined;
  const activeLogsBusy = activeLogsId ? comboBusy[activeLogsId] === "stop" : false;
  const downloadDir = config?.downloadDir ?? "";
  const anyBusy = bulkBusy !== null || savingConfig;
  const allRunning = combos.length > 0 && runningCount === combos.length;
  const editorOpen = editor !== null;

  if (booting) {
    return (
      <div className="boot-screen">
        <Spinner size={20} />
        <span>正在加载…</span>
      </div>
    );
  }

  if (bootError && !config) {
    return (
      <div className="boot-screen boot-error">
        <h1>加载失败</h1>
        <p>{bootError}</p>
        <button
          type="button"
          className="btn btn-primary"
          onClick={() => {
            setBooting(true);
            void loadState(true).finally(() => setBooting(false));
          }}
        >
          重新加载
        </button>
      </div>
    );
  }

  return (
    <div className="app-shell">
      <header className="app-header">
        <div className="app-brand">
          <span className="app-logo">
            <Icon name="terminal" size={20} />
          </span>
          <h1>日志采集工具</h1>
        </div>
        <div className="header-actions">
          <button
            type="button"
            className="btn btn-secondary btn-sm"
            onClick={() => setSettingsOpen(true)}
            disabled={!config || editorOpen || savingConfig}
          >
            设置
          </button>
          <button
            type="button"
            className="btn btn-secondary btn-sm"
            onClick={handleManualSave}
            disabled={!config || savingConfig || editorOpen}
          >
            {savingConfig ? <Spinner /> : null}
            保存配置
          </button>
          <button
            type="button"
            className="btn btn-primary btn-sm"
            onClick={handleAdd}
            disabled={adding || anyBusy || editorOpen}
          >
            {adding ? <Spinner /> : <Icon name="plus" size={14} />}
            新增云主机
          </button>
        </div>
      </header>

      <main className="page">
        {configError ? (
          <div className="alert alert-warning page-alert" role="alert">
            {configError}
          </div>
        ) : null}

        <section className="panel host-panel">
          <div className="panel-head">
            <div className="panel-title">
              <h2>云主机列表</h2>
              <span className="count">{combos.length}</span>
              <span className="summary">
                运行中 {runningCount}
                {incompleteCount > 0 ? (
                  <>
                    <span>·</span>
                    <span className="summary-warning">{incompleteCount} 个待完善</span>
                  </>
                ) : null}
              </span>
            </div>
            <div className="panel-actions">
              <button
                type="button"
                className="btn btn-secondary btn-sm"
                onClick={handleStartAll}
                disabled={anyBusy || editorOpen || combos.length === 0 || allRunning}
              >
                {bulkBusy === "start" ? <Spinner /> : null}
                全部采集
              </button>
              <button
                type="button"
                className="btn btn-secondary btn-sm"
                onClick={handleStopAll}
                disabled={anyBusy || runningCount === 0}
              >
                {bulkBusy === "stop" ? <Spinner /> : null}
                全部停止
              </button>
            </div>
          </div>

          <div className="path-bar">
            <div className="path-text">
              日志保存位置：<code title={downloadDir}>{downloadDir || "未设置"}</code>
            </div>
            <div className="path-actions">
              <button type="button" className="btn btn-text btn-sm" onClick={() => void loadState(false)}>
                刷新状态
              </button>
              <button type="button" className="btn btn-text btn-sm" onClick={handleChooseDownloadDir}>
                更改目录
              </button>
              <button type="button" className="btn btn-text btn-sm" onClick={handleOpenDownloadFolder}>
                打开目录
              </button>
            </div>
          </div>

          {combos.length === 0 ? (
            <div className="empty-state">
              <Icon name="server" size={28} />
              <h3>还没有云主机</h3>
              <p>点击右上角“新增云主机”，填写云主机和设备信息后即可开始采集。</p>
              <button
                type="button"
                className="btn btn-primary btn-sm"
                onClick={handleAdd}
                disabled={adding || anyBusy || editorOpen}
              >
                <Icon name="plus" size={14} />
                新增云主机
              </button>
            </div>
          ) : (
            <div className="host-table">
              <div className="table-head">
                <span>名称</span>
                <span>云主机</span>
                <span>设备 A</span>
                <span>设备 B</span>
                <span>状态</span>
                <span>操作</span>
              </div>
              {combos.map((combo) => (
                <ComboRow
                  key={combo.id}
                  combo={combo}
                  run={runs[combo.id]}
                  busyAction={comboBusy[combo.id] ?? null}
                  disabled={anyBusy || editorOpen}
                  onStart={() => void handleStart(combo.id)}
                  onStop={() => void handleStop(combo.id)}
                  onOpenLogs={() => void openLogs(combo.id)}
                  onEdit={() => handleEdit(combo)}
                  onDelete={() => setDeleteTarget(combo)}
                />
              ))}
            </div>
          )}
        </section>

        {editor ? (
          <ComboEditor
            mode={editor.mode}
            initial={editor.initial}
            onClose={() => setEditor(null)}
            onSave={handleSaveCombo}
          />
        ) : null}

        <footer className="app-footer">
          <span className="config-path" title={configPath}>
            配置文件：<code>{configPath || "未初始化"}</code>
          </span>
          <div className="footer-actions">
            <span className="security-note">账号密码仅保存在本地配置文件</span>
            <button type="button" className="btn btn-text btn-sm" onClick={handleOpenConfigFolder} disabled={!configPath}>
              打开配置位置
            </button>
            <button type="button" className="btn btn-text btn-sm" onClick={copyConfigPath} disabled={!configPath}>
              复制路径
            </button>
          </div>
        </footer>
      </main>

      {config ? (
        <SettingsModal
          open={settingsOpen}
          config={config}
          onClose={() => setSettingsOpen(false)}
          onSave={handleSaveSettings}
        />
      ) : null}

      <LogsPanel
        open={Boolean(activeLogsId && activeLogsCombo)}
        combo={activeLogsCombo}
        run={activeLogsRun}
        logs={logs}
        loading={logsLoading}
        stopBusy={activeLogsBusy}
        onClose={closeLogs}
        onClear={() => void handleClearLogs()}
        onStop={() => {
          if (activeLogsId) void handleStop(activeLogsId);
        }}
        onOpenFolder={() => void handleOpenDownloadFolder()}
      />

      <ConfirmDialog
        open={deleteTarget !== null}
        title="删除云主机"
        message={
          <div>
            <p>确定删除 <strong>{deleteTarget?.name}</strong> 吗？</p>
            <p className="confirm-secondary">只会删除配置，不会删除已下载的日志文件。</p>
          </div>
        }
        confirmLabel={deleting ? "正在删除" : "删除"}
        danger
        busy={deleting}
        onCancel={() => {
          if (!deleting) setDeleteTarget(null);
        }}
        onConfirm={() => void handleDelete()}
      />

      <ToastStack
        toasts={toasts}
        onDismiss={(id) => setToasts((current) => current.filter((toast) => toast.id !== id))}
      />
    </div>
  );
}

export default App;
