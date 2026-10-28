import {
  createContext,
  useContext,
  useEffect,
  useId,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { Icon, type IconName } from "./Icon";

interface ModalProps {
  open: boolean;
  title: string;
  subtitle?: string;
  icon?: IconName;
  size?: "md" | "lg" | "xl";
  onClose: () => void;
  closeDisabled?: boolean;
  footer?: ReactNode;
  children: ReactNode;
}

export function Modal({
  open,
  title,
  subtitle,
  icon,
  size = "md",
  onClose,
  closeDisabled = false,
  footer,
  children,
}: ModalProps) {
  const titleId = useId();
  const panelRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !closeDisabled) {
        event.preventDefault();
        onClose();
      }
    };
    document.addEventListener("keydown", onKeyDown);
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    const focusTimer = window.setTimeout(() => {
      const target = panelRef.current?.querySelector<HTMLElement>(
        "input:not([type='hidden']), textarea, button:not(.modal-close)",
      );
      (target ?? panelRef.current)?.focus();
    }, 40);
    return () => {
      document.removeEventListener("keydown", onKeyDown);
      document.body.style.overflow = previousOverflow;
      window.clearTimeout(focusTimer);
    };
  }, [open, closeDisabled, onClose]);

  if (!open) return null;

  return (
    <div
      className="modal-backdrop"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget && !closeDisabled) onClose();
      }}
    >
      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        tabIndex={-1}
        className={"modal-panel modal-" + size}
      >
        <header className="modal-header">
          <div className="modal-title-wrap">
            {icon ? (
              <span className="modal-icon">
                <Icon name={icon} size={20} />
              </span>
            ) : null}
            <div>
              <h2 id={titleId}>{title}</h2>
              {subtitle ? <p>{subtitle}</p> : null}
            </div>
          </div>
          <button
            type="button"
            className="icon-button modal-close"
            aria-label="关闭"
            onClick={onClose}
            disabled={closeDisabled}
          >
            <Icon name="x" size={19} />
          </button>
        </header>
        <div className="modal-body">{children}</div>
        {footer ? <footer className="modal-footer">{footer}</footer> : null}
      </div>
    </div>
  );
}

interface FieldProps {
  label: string;
  hint?: string;
  required?: boolean;
  className?: string;
  children: ReactNode;
}

export function Field({ label, hint, required = false, className = "", children }: FieldProps) {
  return (
    <label className={"field " + className}>
      <span className="field-label">
        {label}
        {required ? <em aria-hidden="true">*</em> : null}
        {hint ? <small>{hint}</small> : null}
      </span>
      {children}
    </label>
  );
}

interface PasswordInputProps {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  autoComplete?: string;
  name?: string;
}

export function PasswordInput({
  value,
  onChange,
  placeholder = "输入密码",
  autoComplete = "new-password",
  name,
}: PasswordInputProps) {
  const [visible, setVisible] = useState(false);
  return (
    <span className="password-wrap">
      <input
        type={visible ? "text" : "password"}
        value={value}
        name={name}
        autoComplete={autoComplete}
        placeholder={placeholder}
        spellCheck={false}
        onChange={(event) => onChange(event.target.value)}
      />
      <button
        type="button"
        className="password-toggle"
        onClick={() => setVisible((current) => !current)}
        aria-label={visible ? "隐藏密码" : "显示密码"}
        tabIndex={-1}
      >
        <Icon name={visible ? "eye-off" : "eye"} size={16} />
      </button>
    </span>
  );
}

interface SwitchProps {
  checked: boolean;
  onChange: (checked: boolean) => void;
  label: string;
  description?: string;
}

export function Switch({ checked, onChange, label, description }: SwitchProps) {
  return (
    <label className="switch-row">
      <span>
        <strong>{label}</strong>
        {description ? <small>{description}</small> : null}
      </span>
      <button
        type="button"
        role="switch"
        aria-checked={checked}
        className={"switch " + (checked ? "is-on" : "")}
        onClick={() => onChange(!checked)}
      >
        <span />
      </button>
    </label>
  );
}

export function Spinner({ size = 16 }: { size?: number }) {
  return <Icon name="spinner" size={size} className="spinner-icon" />;
}

export type ToastKind = "success" | "error" | "info" | "warning";

export interface ToastItem {
  id: number;
  kind: ToastKind;
  title: string;
  message?: string;
}

const toastIcons: Record<ToastKind, IconName> = {
  success: "check",
  error: "alert",
  info: "info",
  warning: "alert",
};

export function ToastStack({
  toasts,
  onDismiss,
}: {
  toasts: ToastItem[];
  onDismiss: (id: number) => void;
}) {
  return (
    <div className="toast-stack" aria-live="polite">
      {toasts.map((toast) => (
        <div key={toast.id} className={"toast toast-" + toast.kind}>
          <span className="toast-icon">
            <Icon name={toastIcons[toast.kind]} size={18} />
          </span>
          <div className="toast-copy">
            <strong>{toast.title}</strong>
            {toast.message ? <p>{toast.message}</p> : null}
          </div>
          <button
            type="button"
            className="toast-close"
            aria-label="关闭提示"
            onClick={() => onDismiss(toast.id)}
          >
            <Icon name="x" size={15} />
          </button>
        </div>
      ))}
    </div>
  );
}

interface ConfirmDialogProps {
  open: boolean;
  title: string;
  message: ReactNode;
  confirmLabel: string;
  danger?: boolean;
  busy?: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}

export function ConfirmDialog({
  open,
  title,
  message,
  confirmLabel,
  danger = false,
  busy = false,
  onCancel,
  onConfirm,
}: ConfirmDialogProps) {
  return (
    <Modal
      open={open}
      title={title}
      onClose={onCancel}
      closeDisabled={busy}
      size="md"
      footer={
        <>
          <button type="button" className="btn btn-ghost" onClick={onCancel} disabled={busy}>
            取消
          </button>
          <button
            type="button"
            className={"btn " + (danger ? "btn-danger" : "btn-primary")}
            onClick={onConfirm}
            disabled={busy}
          >
            {busy ? <Spinner /> : null}
            {confirmLabel}
          </button>
        </>
      }
    >
      <div className="confirm-copy">{message}</div>
    </Modal>
  );
}

export interface CopyContextValue {
  copy: (text: string) => Promise<boolean>;
}

export const CopyContext = createContext<CopyContextValue>({
  copy: async () => false,
});

export function useCopyFeedback() {
  return useContext(CopyContext);
}
