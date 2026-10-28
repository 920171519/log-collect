import { useEffect, useRef, useState, type FormEvent } from "react";
import type { Combination, DeviceConfig, HostConfig } from "../types";
import { hasValue, joinPaths, splitPaths } from "../utils";
import { Field, PasswordInput, Spinner } from "./ui";

interface ComboEditorProps {
  mode: "create" | "edit";
  initial: Combination;
  onClose: () => void;
  onSave: (combo: Combination) => Promise<void>;
}

type DeviceRole = "A" | "B";

function cloneCombo(combo: Combination): Combination {
  return JSON.parse(JSON.stringify(combo)) as Combination;
}

function portValue(port: number): number | "" {
  return Number.isFinite(port) && port > 0 ? port : "";
}

function validateAddress(value: string): boolean {
  return hasValue(value) && !/\s/.test(value);
}

function validateMarker(marker: string): boolean {
  return hasValue(marker) && !marker.includes("\n") && !marker.includes("\r");
}

export function ComboEditor({ mode, initial, onClose, onSave }: ComboEditorProps) {
  const [draft, setDraft] = useState<Combination>(() => cloneCombo(initial));
  const [pathAText, setPathAText] = useState("");
  const [pathBText, setPathBText] = useState("");
  const [bEnabled, setBEnabled] = useState(false);
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const sectionRef = useRef<HTMLElement>(null);

  useEffect(() => {
    const next = cloneCombo(initial);
    setDraft(next);
    setPathAText(joinPaths(next.deviceA.collectPaths));
    setPathBText(joinPaths(next.deviceB.collectPaths));
    setBEnabled(hasValue(next.deviceB.ip));
    setError("");
    setSaving(false);
    window.setTimeout(() => {
      sectionRef.current?.scrollIntoView({ behavior: "smooth", block: "start" });
    }, 50);
  }, [initial]);

  const clearError = () => {
    if (error) setError("");
  };

  const updateCloud = (patch: Partial<HostConfig>) => {
    clearError();
    setDraft((current) => ({ ...current, cloud: { ...current.cloud, ...patch } }));
  };

  const updateDevice = (role: DeviceRole, patch: Partial<DeviceConfig>) => {
    clearError();
    setDraft((current) => {
      const key = role === "A" ? "deviceA" : "deviceB";
      return { ...current, [key]: { ...current[key], ...patch } };
    });
  };

  const buildDevice = (device: DeviceConfig, pathText: string, enabled = true): DeviceConfig => {
    const next: DeviceConfig = {
      ...device,
      ip: enabled ? device.ip.trim() : "",
      port: Number(device.port),
      username: device.username.trim(),
      password: device.password,
      commands: device.commands,
      collectPaths: splitPaths(pathText),
      endMarker: device.endMarker.trim(),
    };
    return next;
  };

  const buildCombo = (): Combination => ({
    ...draft,
    name: draft.name.trim() || draft.cloud.ip.trim() || "未命名组合",
    cloud: {
      ...draft.cloud,
      ip: draft.cloud.ip.trim(),
      port: Number(draft.cloud.port),
      username: draft.cloud.username.trim(),
      password: draft.cloud.password,
    },
    deviceA: buildDevice(draft.deviceA, pathAText),
    deviceB: buildDevice(draft.deviceB, pathBText, bEnabled),
  });

  const validate = (combo: Combination): string => {
    if (!validateAddress(combo.cloud.ip)) return "请填写云主机 IP。";
    if (!(combo.cloud.port > 0 && combo.cloud.port <= 65535)) return "云主机 SSH 端口应为 1-65535。";
    if (!hasValue(combo.cloud.username)) return "请填写云主机用户名。";
    if (!hasValue(combo.cloud.password)) return "请填写云主机密码。";
    if (!validateAddress(combo.deviceA.ip)) return "请填写设备 A 的 IP。";
    if (!(combo.deviceA.port > 0 && combo.deviceA.port <= 65535)) return "设备 A 的 SSH 端口应为 1-65535。";
    if (!hasValue(combo.deviceA.username)) return "请填写设备 A 的用户名。";
    if (!hasValue(combo.deviceA.password)) return "请填写设备 A 的密码。";
    if (!hasValue(combo.deviceA.commands)) return "请填写设备 A 的采集命令。";
    if (!validateMarker(combo.deviceA.endMarker)) return "设备 A 的结束标记必须是非空单行文本。";
    if (combo.deviceA.collectPaths.length === 0) return "请至少填写一个设备 A 的日志路径。";
    if (bEnabled) {
      if (!validateAddress(combo.deviceB.ip)) return "请填写设备 B 的 IP，或取消勾选「接入设备 B」。";
      if (!(combo.deviceB.port > 0 && combo.deviceB.port <= 65535)) return "设备 B 的 SSH 端口应为 1-65535。";
      if (!hasValue(combo.deviceB.username)) return "请填写设备 B 的用户名。";
      if (!hasValue(combo.deviceB.password)) return "请填写设备 B 的密码。";
      if (!hasValue(combo.deviceB.commands)) return "请填写设备 B 的采集命令。";
      if (!validateMarker(combo.deviceB.endMarker)) return "设备 B 的结束标记必须是非空单行文本。";
      if (combo.deviceB.collectPaths.length === 0) return "请至少填写一个设备 B 的日志路径。";
    }
    return "";
  };

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (saving) return;
    const combo = buildCombo();
    const validation = validate(combo);
    if (validation) {
      setError(validation);
      sectionRef.current?.scrollIntoView({ behavior: "smooth", block: "start" });
      return;
    }
    setError("");
    setSaving(true);
    try {
      await onSave(combo);
    } catch (saveError) {
      setError(saveError instanceof Error ? saveError.message : String(saveError));
    } finally {
      setSaving(false);
    }
  };

  const renderDeviceFields = (role: DeviceRole) => {
    const device = role === "A" ? draft.deviceA : draft.deviceB;
    const pathText = role === "A" ? pathAText : pathBText;
    const setPathText = role === "A" ? setPathAText : setPathBText;
    return (
      <div className="field-grid cols-4">
        <Field label={"设备 " + role + " IP"} required>
          <input
            value={device.ip}
            spellCheck={false}
            autoComplete="off"
            placeholder={role === "A" ? "例如 192.168.1.10" : "例如 192.168.1.20"}
            onChange={(event) => updateDevice(role, { ip: event.target.value })}
          />
        </Field>
        <Field label="SSH 端口" required>
          <input
            type="number"
            min={1}
            max={65535}
            value={portValue(device.port)}
            onChange={(event) => updateDevice(role, { port: Number(event.target.value) })}
          />
        </Field>
        <Field label="用户名" required>
          <input
            value={device.username}
            autoComplete="off"
            spellCheck={false}
            placeholder="例如 root"
            onChange={(event) => updateDevice(role, { username: event.target.value })}
          />
        </Field>
        <Field label="密码" required>
          <PasswordInput
            value={device.password}
            onChange={(password) => updateDevice(role, { password })}
            placeholder="设备 SSH 密码"
          />
        </Field>
        <Field label="日志路径（每行一个）" required className="span-2">
          <textarea
            rows={3}
            value={pathText}
            spellCheck={false}
            placeholder={"/var/log/app\n/tmp/diag"}
            onChange={(event) => {
              clearError();
              setPathText(event.target.value);
            }}
          />
        </Field>
        <Field label="结束标记" required className="span-2" hint="通常不需要修改">
          <input
            value={device.endMarker}
            spellCheck={false}
            autoComplete="off"
            placeholder="__LOGCOLLECT_DONE__"
            onChange={(event) => updateDevice(role, { endMarker: event.target.value })}
          />
        </Field>
        <Field label="采集命令" required className="span-4">
          <textarea
            rows={6}
            className="code-input"
            value={device.commands}
            spellCheck={false}
            placeholder="输入要在设备上执行的 shell 命令"
            onChange={(event) => updateDevice(role, { commands: event.target.value })}
          />
          <span className="field-help">
            命令执行完成后，程序会自动追加结束标记，检测到标记后再打包并下载日志。请勿在命令中使用 exit 提前退出。
          </span>
        </Field>
      </div>
    );
  };

  return (
    <section className="panel inline-editor" ref={sectionRef} aria-label={mode === "create" ? "新增云主机" : "编辑云主机"}>
      <div className="editor-head">
        <div>
          <h2>{mode === "create" ? "新增云主机" : "编辑云主机"}</h2>
          <p>带 * 的为必填项。保存后会出现在上方云主机列表中。</p>
        </div>
        <button type="button" className="btn btn-secondary btn-sm" onClick={onClose} disabled={saving}>
          取消
        </button>
      </div>

      <form className="editor-form" onSubmit={submit}>
        {error ? (
          <div className="alert alert-error" role="alert">
            {error}
          </div>
        ) : null}

        <section className="form-block">
          <h3>云主机</h3>
          <div className="field-grid cols-4">
            <Field label="名称" className="span-2" hint="选填，默认使用云主机 IP">
              <input
                value={draft.name}
                placeholder="例如：机房 A 采集服务器"
                onChange={(event) => {
                  clearError();
                  setDraft({ ...draft, name: event.target.value });
                }}
              />
            </Field>
            <Field label="云主机 IP" required>
              <input
                value={draft.cloud.ip}
                spellCheck={false}
                autoComplete="off"
                placeholder="例如 10.0.0.10"
                onChange={(event) => updateCloud({ ip: event.target.value })}
              />
            </Field>
            <Field label="SSH 端口" required>
              <input
                type="number"
                min={1}
                max={65535}
                value={portValue(draft.cloud.port)}
                onChange={(event) => updateCloud({ port: Number(event.target.value) })}
              />
            </Field>
            <Field label="用户名" required className="span-2">
              <input
                value={draft.cloud.username}
                autoComplete="off"
                spellCheck={false}
                placeholder="例如 root"
                onChange={(event) => updateCloud({ username: event.target.value })}
              />
            </Field>
            <Field label="密码" required className="span-2">
              <PasswordInput
                value={draft.cloud.password}
                onChange={(password) => updateCloud({ password })}
                placeholder="云主机 SSH 密码"
              />
            </Field>
          </div>
        </section>

        <section className="form-block">
          <div className="form-block-head">
            <h3>设备 A</h3>
            <span className="required-note">必须填写</span>
          </div>
          {renderDeviceFields("A")}
        </section>

        <section className="form-block">
          <div className="form-block-head">
            <h3>设备 B</h3>
            <label className="check-inline">
              <input
                type="checkbox"
                checked={bEnabled}
                onChange={(event) => {
                  clearError();
                  setBEnabled(event.target.checked);
                }}
              />
              接入设备 B（可选）
            </label>
          </div>
          {bEnabled ? (
            <div className="device-b-fields">
              <p className="form-tip">
                设备 B 无需在云主机上直接可达。程序会先连接设备 A，再通过设备 A 的 SSH 通道连接 B 并执行下面的命令。
              </p>
              {renderDeviceFields("B")}
            </div>
          ) : (
            <p className="form-tip muted-tip">如果云主机没有连接设备 B，可以不勾选，仅采集设备 A。</p>
          )}
        </section>

        <div className="editor-actions">
          <button type="button" className="btn btn-secondary" onClick={onClose} disabled={saving}>
            取消
          </button>
          <button type="submit" className="btn btn-primary" disabled={saving}>
            {saving ? <Spinner /> : null}
            {saving ? "保存中…" : mode === "create" ? "保存并添加" : "保存修改"}
          </button>
        </div>
      </form>
    </section>
  );
}
