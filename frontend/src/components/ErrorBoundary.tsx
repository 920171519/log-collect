import React from "react";

interface ErrorBoundaryState {
  error: Error | null;
}

export class ErrorBoundary extends React.Component<
  { children: React.ReactNode },
  ErrorBoundaryState
> {
  state: ErrorBoundaryState = { error: null };

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { error };
  }

  componentDidCatch(error: Error, info: React.ErrorInfo) {
    console.error("LogCollector UI error", error, info);
  }

  render() {
    if (this.state.error) {
      return (
        <div className="crash-screen">
          <div className="crash-card">
            <span className="crash-kicker">UI ERROR</span>
            <h1>界面遇到了一点问题</h1>
            <p>采集任务可能仍在后台运行。重新加载不会修改配置文件。</p>
            <pre>{this.state.error.message}</pre>
            <button type="button" className="btn btn-primary" onClick={() => window.location.reload()}>
              重新加载界面
            </button>
          </div>
        </div>
      );
    }
    return this.props.children;
  }
}
