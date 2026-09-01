import type {
  ActionResult,
  ActivityEvent,
  Alert,
  CheckReport,
  MetricsSnapshot,
  Overview,
  PruneConfig,
  PruneRun,
  PruneStatus,
  Resources,
  StackView,
} from "./types";

// The key is only needed when the server runs with API_KEY set.
const API_KEY_STORAGE = "ops-console.apiKey";

export function apiKey(): string {
  return localStorage.getItem(API_KEY_STORAGE) ?? "";
}

export function setApiKey(key: string) {
  if (key) localStorage.setItem(API_KEY_STORAGE, key);
  else localStorage.removeItem(API_KEY_STORAGE);
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers);
  headers.set("Accept", "application/json");
  if (init?.body) headers.set("Content-Type", "application/json");
  const key = apiKey();
  if (key) headers.set("X-API-Key", key);

  const res = await fetch(`/api${path}`, { ...init, headers });
  const text = await res.text();
  const body = text ? JSON.parse(text) : {};
  if (!res.ok) {
    throw new Error(body?.error ?? `${res.status} ${res.statusText}`);
  }
  return body as T;
}

export const api = {
  overview: (refresh = false) =>
    request<Overview>(`/overview${refresh ? "?refresh=1" : ""}`),
  stacks: () => request<{ stacks: StackView[] }>("/stacks"),
  stack: (name: string) => request<StackView>(`/stacks/${encodeURIComponent(name)}`),
  getCompose: (name: string) =>
    request<{ path: string; content: string }>(`/stacks/${encodeURIComponent(name)}/compose`),
  saveCompose: (name: string, content: string) =>
    request<{ path: string }>(`/stacks/${encodeURIComponent(name)}/compose`, {
      method: "PUT",
      body: JSON.stringify({ content }),
    }),
  stackAction: (name: string, action: "up" | "down" | "restart" | "pull" | "rebuild") =>
    request<ActionResult>(`/stacks/${encodeURIComponent(name)}/${action}`, { method: "POST" }),
  containerAction: (id: string, action: "start" | "stop" | "restart") =>
    request<{ ok: boolean }>(`/containers/${encodeURIComponent(id)}/${action}`, { method: "POST" }),
  containerLogs: (id: string, tail = 200) =>
    request<{ logs: string }>(`/containers/${encodeURIComponent(id)}/logs?tail=${tail}`),
  metrics: () => request<MetricsSnapshot>("/metrics"),
  activity: (limit = 50) => request<{ events: ActivityEvent[] }>(`/activity?limit=${limit}`),
  alerts: () => request<{ alerts: Alert[] }>("/alerts"),
  checks: () => request<CheckReport>("/checks"),
  resources: () => request<Resources>("/resources"),
  pruneStatus: () => request<PruneStatus>("/prune/status"),
  pruneHistory: () => request<{ history: PruneRun[] }>("/prune/history"),
  savePruneConfig: (config: PruneConfig) =>
    request<{ config: PruneConfig; status: PruneStatus }>("/prune/config", {
      method: "PUT",
      body: JSON.stringify(config),
    }),
  runPrune: (dryRun: boolean, override?: PruneConfig) =>
    request<{ run: PruneRun; status: PruneStatus }>("/prune/run", {
      method: "POST",
      body: JSON.stringify({ dryRun, override }),
    }),
};
