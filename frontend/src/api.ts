import {
  ChooseDownloadDir as goChooseDownloadDir,
  ClearLogs as goClearLogs,
  GetAppState as goGetAppState,
  GetLogs as goGetLogs,
  NewCombinationDraft as goNewCombinationDraft,
  OpenConfigFolder as goOpenConfigFolder,
  OpenDownloadFolder as goOpenDownloadFolder,
  SaveConfig as goSaveConfig,
  StartAll as goStartAll,
  StartCombination as goStartCombination,
  StopAll as goStopAll,
  StopCombination as goStopCombination,
} from "../wailsjs/go/main/App";
import type {
  AppConfig,
  AppState,
  Combination,
  LogEntry,
} from "./types";

// The generated Wails bindings use generated model classes. The UI intentionally
// keeps plain JSON interfaces; these wrappers are the single cast boundary.
export async function getAppState(): Promise<AppState> {
  return (await goGetAppState()) as unknown as AppState;
}

export async function saveConfig(config: AppConfig): Promise<AppConfig> {
  return (await goSaveConfig(config as never)) as unknown as AppConfig;
}

export async function newCombinationDraft(): Promise<Combination> {
  return (await goNewCombinationDraft()) as unknown as Combination;
}

export async function startCombination(comboId: string): Promise<void> {
  await goStartCombination(comboId);
}

export async function startAll(): Promise<number> {
  return goStartAll();
}

export async function stopCombination(comboId: string): Promise<void> {
  await goStopCombination(comboId);
}

export async function stopAll(): Promise<number> {
  return goStopAll();
}

export async function getLogs(comboId: string): Promise<LogEntry[]> {
  return (await goGetLogs(comboId)) as unknown as LogEntry[];
}

export async function clearLogs(comboId: string): Promise<void> {
  await goClearLogs(comboId);
}

export async function chooseDownloadDir(current: string): Promise<string> {
  return goChooseDownloadDir(current);
}

export async function openDownloadFolder(): Promise<void> {
  await goOpenDownloadFolder();
}

export async function openConfigFolder(): Promise<void> {
  await goOpenConfigFolder();
}
