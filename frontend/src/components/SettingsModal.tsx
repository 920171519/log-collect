import { useEffect, useState, type FormEvent } from "react";
import { chooseDownloadDir } from "../api";
import type { AppConfig } from "../types";
import { errorMessage, hasValue } from "../utils";
import { Field, Modal, Spinner } from "./ui";

interface SettingsModalProps {
  open: boolean;
  config: AppConfig;
  onClose: () => void;
  onSave: (config: AppConfig) => Promise<void>;
}

export function SettingsModal({ open, config, onClose, onSave }: SettingsModalProps) {
  const [draft, setDraft] = useState<AppConfig>(config);
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const [choosing, setChoosing] = useState(false);

  useEffect(() => {
    if (!open) return;
    setDraft(config);
    setError("");
    setSaving(false);
    setChoosing(false);
  }, [open, config]);

  const update = (patch: Partial<AppConfig>) => {
    setDraft((current) => ({ ...current, ...patch }));
  };

  const chooseDir = async () => {
    setChoosing(true);
    try {
      const selected = await chooseDownloadDir(draft.downloadDir);
      if (selected) update({ downloadDir: selected });
    } catch (chooseError) {
      setError(errorMessage(chooseError));
    } finally {
      setChoosing(false);
    }
  };

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (saving) return;
    if (!hasValue(draft.downloadDir)) {
      setError("请选择本地下载目录。");
      return;
    }
    if (!draft.archiveNameTemplate.includes("{device}")) {
      setError("日志文件名必须包含 {device}，防止设备 A/B 归档重名。");
      return;
    }
    if (!draft.cloudStageDir.startsWith("/") || !draft.deviceTempDir.startsWith("/")) {
      setError("云主机暂存目录和设备临时目录必须是 Linux 绝对路径。");
      return;
    }
    if (!(draft.timeoutMinutes >= 1 && draft.timeoutMinutes <= 1440)) {
      setError("超时时间应在 1-1440 分钟之间。");
      return;
    }
    setError("");
    setSaving(true);
    try {
      await onSave({
        ...draft,
        downloadDir: draft.downloadDir.trim(),
        archiveNameTemplate: draft.archiveNameTemplate.trim(),
        cloudStageDir: draft.cloudStageDir.trim(),
        deviceTempDir: draft.deviceTempDir.trim(),
        timeoutMinutes: Number(draft.timeoutMinutes),
      });
    } catch (saveError) {
      setError(errorMessage(saveError));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal
      open={open}
      title="设置"
      subtitle="修改日志保存位置、远端临时目录和采集超时"
      size="lg"
      onClose={onClose}
      closeDisabled={saving}
      footer={
        <div className="modal-footer-row">
          <button type="button" className="btn btn-secondary" onClick={onClose} disabled={saving}>
            取消
          </button>
          <button type="submit" form="settings-form" className="btn btn-primary" disabled={saving}>
            {saving ? <Spinner /> : null}
            {saving ? "保存中…" : "保存设置"}
          </button>
        </div>
      }
    >
      <form id="settings-form" className="settings-form" onSubmit={submit}>
        {error ? <div className="alert alert-error">{error}</div> : null}

        <section className="form-block">
          <h3>本地保存位置</h3>
          <Field label="日志下载目录">
            <div className="input-with-button">
              <input
                value={draft.downloadDir}
                spellCheck={false}
                onChange={(event) => update({ downloadDir: event.target.value })}
              />
              <button type="button" className="btn btn-secondary" onClick={chooseDir} disabled={choosing}>
                {choosing ? <Spinner /> : null}
                选择目录
              </button>
            </div>
          </Field>
        </section>

        <section className="form-block">
          <h3>文件传输</h3>
          <div className="field-grid cols-2">
            <Field label="日志文件名" hint="支持 {combo} {device} {timestamp} {date}" className="span-2">
              <input
                value={draft.archiveNameTemplate}
                spellCheck={false}
                onChange={(event) => update({ archiveNameTemplate: event.target.value })}
              />
            </Field>
            <Field label="云主机暂存目录" hint="Linux 绝对路径">
              <input
                value={draft.cloudStageDir}
                spellCheck={false}
                onChange={(event) => update({ cloudStageDir: event.target.value })}
              />
            </Field>
            <Field label="设备临时目录" hint="Linux 绝对路径">
              <input
                value={draft.deviceTempDir}
                spellCheck={false}
                onChange={(event) => update({ deviceTempDir: event.target.value })}
              />
            </Field>
          </div>
          <label className="check-line settings-check">
            <input
              type="checkbox"
              checked={draft.keepCloudCopy}
              onChange={(event) => update({ keepCloudCopy: event.target.checked })}
            />
            下载完成后保留云主机上的日志文件
          </label>
          <p className="form-tip">
            设备需要可用 sh、tar 和 SFTP；传输使用内置 SSH/SFTP，不需要安装 scp 或 sshpass。
          </p>
        </section>

        <section className="form-block">
          <h3>任务</h3>
          <Field label="单次采集超时" hint="1-1440 分钟">
            <input
              type="number"
              min={1}
              max={1440}
              value={draft.timeoutMinutes || ""}
              onChange={(event) => update({ timeoutMinutes: Number(event.target.value) })}
            />
          </Field>
        </section>
      </form>
    </Modal>
  );
}
