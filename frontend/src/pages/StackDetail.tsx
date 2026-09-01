import { type ReactNode, useCallback, useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import {
  ArrowLeft,
  Download,
  FileCode2,
  Hammer,
  Play,
  RefreshCw,
  RotateCcw,
  ScrollText,
  Square,
} from "lucide-react";
import { api } from "../api";
import { bytes } from "../format";
import { useAsync } from "../hooks";
import {
  Bar,
  Card,
  Chip,
  Empty,
  ErrorBanner,
  Spinner,
  StateBadge,
} from "../components/ui";
import type { ContainerView, StackView } from "../types";

type Tab = "overview" | "compose" | "logs";

export default function StackDetail() {
  const { name = "" } = useParams();
  const decoded = decodeURIComponent(name);
  const { data, error, loading, reload } = useAsync(
    () => api.stack(decoded),
    5000,
  );
  const [tab, setTab] = useState<Tab>("overview");
  const [busy, setBusy] = useState<string | null>(null);
  const [msg, setMsg] = useState<string | null>(null);
  const [compose, setCompose] = useState<{ path: string; content: string } | null>(null);
  const [logs, setLogs] = useState<{ container: ContainerView; text: string } | null>(null);
  const [logTail, setLogTail] = useState(300);
  const [autoLogs, setAutoLogs] = useState(false);

  const runAction = useCallback(
    async (action: "up" | "down" | "restart" | "pull" | "rebuild") => {
      if (!decoded || decoded === "(standalone)") return;
      setBusy(action);
      setMsg(null);
      try {
        const res = await api.stackAction(decoded, action);
        setMsg(res.output || `${action} completed`);
        await reload();
      } catch (e) {
        setMsg(e instanceof Error ? e.message : String(e));
      } finally {
        setBusy(null);
      }
    },
    [decoded, reload],
  );

  const loadCompose = useCallback(async () => {
    if (!decoded || decoded === "(standalone)") return;
    setBusy("compose");
    try {
      const res = await api.getCompose(decoded);
      setCompose({ path: res.path, content: res.content });
      setTab("compose");
    } catch (e) {
      setMsg(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(null);
    }
  }, [decoded]);

  const saveCompose = async () => {
    if (!compose) return;
    setBusy("save");
    try {
      await api.saveCompose(decoded, compose.content);
      setMsg(`Saved ${compose.path}`);
    } catch (e) {
      setMsg(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(null);
    }
  };

  const loadLogs = useCallback(
    async (c: ContainerView) => {
      setBusy("logs");
      try {
        const res = await api.containerLogs(c.id, logTail);
        setLogs({ container: c, text: res.logs || "(empty)" });
        setTab("logs");
      } catch (e) {
        setMsg(e instanceof Error ? e.message : String(e));
      } finally {
        setBusy(null);
      }
    },
    [logTail],
  );

  useEffect(() => {
    if (!autoLogs || !logs) return;
    const id = window.setInterval(() => void loadLogs(logs.container), 5000);
    return () => window.clearInterval(id);
  }, [autoLogs, logs, loadLogs]);

  const containerAction = async (id: string, action: "start" | "stop" | "restart") => {
    setBusy(`${id}:${action}`);
    try {
      await api.containerAction(id, action);
      await reload();
    } catch (e) {
      setMsg(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(null);
    }
  };

  if (loading && !data) return <Spinner label="Loading stack" />;
  if (error && !data) return <ErrorBanner message={error} />;
  if (!data) return <Empty>Stack not found.</Empty>;

  const stack = data;
  const isStandalone = stack.name === "(standalone)";
  const offline = !stack.deployed || stack.running === 0;
  const degraded = stack.deployed && stack.running < stack.total;

  return (
    <div className="space-y-6 text-base">
      <header className="space-y-4">
        <Link to="/stacks" className="inline-flex items-center gap-1 text-sm text-slate-500 hover:text-slate-300">
          <ArrowLeft size={16} /> All stacks
        </Link>
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <h1 className="text-2xl font-semibold text-slate-100">{stack.name}</h1>
            <p className="mt-1 font-mono text-sm text-slate-500">
              {stack.workingDir || stack.configFiles.join(", ") || "no compose path"}
            </p>
            <p className="mt-2 text-base text-slate-400">
              {offline ? (
                <span className="text-slate-400">Offline — not running</span>
              ) : (
                <span className={degraded ? "text-amber-300" : "text-emerald-300"}>
                  {stack.running}/{stack.total} running
                </span>
              )}
              {(stack.cpuPct ?? 0) > 0 && (
                <span className="ml-3">CPU {stack.cpuPct!.toFixed(1)}%</span>
              )}
              {(stack.memUsage ?? 0) > 0 && (
                <span className="ml-3">RAM {bytes(stack.memUsage)}</span>
              )}
            </p>
          </div>
          {!isStandalone && (
            <div className="flex flex-wrap gap-2">
              <ActionBtn icon={<Play size={16} />} label="Up" busy={busy} action="up" onClick={runAction} primary={offline} />
              <ActionBtn icon={<Square size={16} />} label="Down" busy={busy} action="down" onClick={runAction} />
              <ActionBtn icon={<RotateCcw size={16} />} label="Restart" busy={busy} action="restart" onClick={runAction} />
              <ActionBtn icon={<Hammer size={16} />} label="Rebuild" busy={busy} action="rebuild" onClick={runAction} />
              <ActionBtn icon={<Download size={16} />} label="Pull" busy={busy} action="pull" onClick={runAction} />
              <button className="btn btn-ghost text-base" disabled={!!busy} onClick={loadCompose}>
                <FileCode2 size={16} /> Compose
              </button>
              <button className="btn btn-ghost" onClick={reload}>
                <RefreshCw size={16} />
              </button>
            </div>
          )}
        </div>
      </header>

      {offline && !isStandalone && (
        <div className="rounded-xl border border-slate-600/50 bg-slate-500/10 px-5 py-4 text-base text-slate-300">
          This stack is offline. Press <strong className="text-slate-100">Up</strong> to start all services from the compose file.
        </div>
      )}

      {msg && (
        <div className="rounded-lg border border-ink-700 bg-ink-900/80 px-4 py-3 text-base text-slate-300 whitespace-pre-wrap">
          {msg}
          <button className="ml-3 text-xs text-slate-500 underline" onClick={() => setMsg(null)}>
            dismiss
          </button>
        </div>
      )}

      <div className="flex gap-1 border-b border-ink-800">
        {(["overview", "compose", "logs"] as Tab[]).map((t) => (
          <button
            key={t}
            onClick={() => {
              setTab(t);
              if (t === "compose" && !compose) void loadCompose();
            }}
            className={`px-4 py-2.5 text-base capitalize transition ${
              tab === t
                ? "border-b-2 border-sky-400 text-sky-200"
                : "text-slate-500 hover:text-slate-300"
            }`}
          >
            {t}
          </button>
        ))}
      </div>

      {tab === "overview" && <OverviewTab stack={stack} busy={busy} onLogs={loadLogs} onContainer={containerAction} />}
      {tab === "compose" && (
        <ComposeTab
          stack={stack}
          compose={compose}
          busy={!!busy}
          onChange={(content) => setCompose((c) => (c ? { ...c, content } : null))}
          onSave={saveCompose}
          onSaveUp={async () => {
            await saveCompose();
            await runAction("up");
          }}
          onLoad={loadCompose}
        />
      )}
      {tab === "logs" && (
        <LogsTab
          stack={stack}
          logs={logs}
          tail={logTail}
          auto={autoLogs}
          onTail={setLogTail}
          onAuto={setAutoLogs}
          onSelect={loadLogs}
          onRefresh={() => logs && loadLogs(logs.container)}
        />
      )}
    </div>
  );
}

function ActionBtn({
  icon,
  label,
  action,
  busy,
  onClick,
  primary,
}: {
  icon: ReactNode;
  label: string;
  action: string;
  busy: string | null;
  onClick: (a: "up" | "down" | "restart" | "pull" | "rebuild") => void;
  primary?: boolean;
}) {
  return (
    <button
      className={`btn ${primary ? "btn-primary" : "btn-ghost"}`}
      disabled={!!busy}
      onClick={() => onClick(action as "up" | "down" | "restart" | "pull" | "rebuild")}
    >
      {icon}
      {busy === action ? "…" : label}
    </button>
  );
}

function OverviewTab({
  stack,
  busy,
  onLogs,
  onContainer,
}: {
  stack: StackView;
  busy: string | null;
  onLogs: (c: ContainerView) => void;
  onContainer: (id: string, action: "start" | "stop" | "restart") => void;
}) {
  return (
    <div className="grid gap-4 lg:grid-cols-3">
      <div className="space-y-4 lg:col-span-2">
        <Card title="Containers">
          <div className="space-y-3">
            {stack.containers.length === 0 ? (
              <Empty>No containers yet. Use Up to create and start them from compose.</Empty>
            ) : (
              stack.containers.map((c) => (
              <div
                key={c.id}
                className={`rounded-lg border p-4 ${
                  c.state === "running"
                    ? "border-ink-800 bg-ink-850/50"
                    : "border-amber-500/30 bg-amber-500/5"
                }`}
              >
                <div className="flex flex-wrap items-center gap-2">
                  <StateBadge state={c.state} health={c.health} />
                  <span className="text-base font-medium text-slate-100">{c.name}</span>
                  <span className="font-mono text-sm text-slate-500">{c.image}</span>
                  <div className="ml-auto flex gap-1">
                    <button className="btn btn-ghost !px-2.5 !py-1.5" title="Logs" onClick={() => onLogs(c)}>
                      <ScrollText size={16} />
                    </button>
                    {c.state === "running" ? (
                      <>
                        <button className="btn btn-ghost !px-2.5 !py-1.5" disabled={!!busy} onClick={() => onContainer(c.id, "restart")}>
                          <RotateCcw size={16} />
                        </button>
                        <button className="btn btn-ghost !px-2.5 !py-1.5" disabled={!!busy} onClick={() => onContainer(c.id, "stop")}>
                          <Square size={16} />
                        </button>
                      </>
                    ) : (
                      <button className="btn btn-primary !px-2.5 !py-1.5" disabled={!!busy} onClick={() => onContainer(c.id, "start")}>
                        <Play size={16} /> Start
                      </button>
                    )}
                  </div>
                </div>
                {c.state === "running" && (c.cpuPct ?? 0) > 0 && (
                  <div className="mt-2 grid gap-2 sm:grid-cols-2">
                    <div>
                      <div className="mb-1 flex justify-between text-sm text-slate-500">
                        <span>CPU</span>
                        <span>{c.cpuPct!.toFixed(1)}%</span>
                      </div>
                      <Bar ratio={Math.min(c.cpuPct! / 100, 1)} />
                    </div>
                    <div>
                      <div className="mb-1 flex justify-between text-sm text-slate-500">
                        <span>Memory</span>
                        <span>
                          {bytes(c.memUsage)} {c.memPct ? `(${c.memPct.toFixed(0)}%)` : ""}
                        </span>
                      </div>
                      <Bar ratio={Math.min((c.memPct ?? 0) / 100, 1)} />
                    </div>
                  </div>
                )}
                <div className="mt-2 flex flex-wrap gap-1.5">
                  {c.ports.map((p) => (
                    <Chip key={`${p.public}-${p.proto}`} tone={p.exposed ? "danger" : "muted"}>
                      {p.exposed ? "0.0.0.0" : p.ip}:{p.public}→{p.private}
                    </Chip>
                  ))}
                  {c.restartCount > 0 && <Chip>restarts: {c.restartCount}</Chip>}
                  {c.privileged && <Chip tone="danger">privileged</Chip>}
                </div>
                {c.mounts.length > 0 && (
                  <ul className="mt-2 space-y-0.5 border-t border-ink-800 pt-2 font-mono text-[10px] text-slate-500">
                    {c.mounts.map((m) => (
                      <li key={m.destination}>
                        {m.type === "volume" ? m.name : m.source} → {m.destination}
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            ))
            )}
          </div>
        </Card>
      </div>
      <div className="space-y-4">
        <Card title="Paths">
          <dl className="space-y-2 text-sm">
            <div>
              <dt className="text-xs text-slate-500">Working dir</dt>
              <dd className="font-mono text-xs text-slate-300 break-all">{stack.workingDir || "—"}</dd>
            </div>
            {stack.configFiles.map((f) => (
              <div key={f}>
                <dt className="text-xs text-slate-500">Compose</dt>
                <dd className="font-mono text-xs text-slate-300 break-all">{f}</dd>
              </div>
            ))}
          </dl>
        </Card>
        <Card title="Networks">
          <div className="flex flex-wrap gap-1.5">
            {stack.networks.length === 0 ? (
              <span className="text-sm text-slate-500">none</span>
            ) : (
              stack.networks.map((n) => <Chip key={n}>{n}</Chip>)
            )}
          </div>
        </Card>
        <Card title="Volumes">
          <div className="flex flex-wrap gap-1.5">
            {stack.volumes.length === 0 ? (
              <span className="text-sm text-slate-500">none</span>
            ) : (
              stack.volumes.map((v) => (
                <Chip key={v} tone="muted">
                  {v}
                </Chip>
              ))
            )}
          </div>
        </Card>
      </div>
    </div>
  );
}

function ComposeTab({
  stack,
  compose,
  busy,
  onChange,
  onSave,
  onSaveUp,
  onLoad,
}: {
  stack: StackView;
  compose: { path: string; content: string } | null;
  busy: boolean;
  onChange: (c: string) => void;
  onSave: () => void;
  onSaveUp: () => void;
  onLoad: () => void;
}) {
  if (stack.name === "(standalone)") {
    return <Empty>Standalone containers have no compose file.</Empty>;
  }
  if (!compose) {
    return (
      <Empty>
        <button className="btn btn-primary mt-2" onClick={onLoad}>
          Load compose file
        </button>
      </Empty>
    );
  }
  return (
    <Card title="Compose editor" subtitle={compose.path}>
      <textarea
        className="h-[28rem] w-full rounded-lg border border-ink-700 bg-ink-950 p-3 font-mono text-xs text-slate-200"
        value={compose.content}
        onChange={(e) => onChange(e.target.value)}
        spellCheck={false}
      />
      <div className="mt-3 flex justify-end gap-2">
        <button className="btn btn-ghost" disabled={busy} onClick={onSave}>
          Save
        </button>
        <button className="btn btn-primary" disabled={busy} onClick={onSaveUp}>
          Save &amp; Up
        </button>
      </div>
    </Card>
  );
}

function LogsTab({
  stack,
  logs,
  tail,
  auto,
  onTail,
  onAuto,
  onSelect,
  onRefresh,
}: {
  stack: StackView;
  logs: { container: ContainerView; text: string } | null;
  tail: number;
  auto: boolean;
  onTail: (n: number) => void;
  onAuto: (v: boolean) => void;
  onSelect: (c: ContainerView) => void;
  onRefresh: () => void;
}) {
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-3">
        <label className="text-xs text-slate-400">
          Container
          <select
            className="input ml-2 w-auto"
            value={logs?.container.id ?? ""}
            onChange={(e) => {
              const c = stack.containers.find((x) => x.id === e.target.value);
              if (c) onSelect(c);
            }}
          >
            <option value="">Select…</option>
            {stack.containers.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name} ({c.state})
              </option>
            ))}
          </select>
        </label>
        <label className="text-xs text-slate-400">
          Lines
          <select className="input ml-2 w-20" value={tail} onChange={(e) => onTail(Number(e.target.value))}>
            {[100, 200, 300, 500, 1000].map((n) => (
              <option key={n} value={n}>
                {n}
              </option>
            ))}
          </select>
        </label>
        <label className="flex items-center gap-2 text-xs text-slate-400">
          <input type="checkbox" checked={auto} onChange={(e) => onAuto(e.target.checked)} />
          Auto-refresh (5s)
        </label>
        <button className="btn btn-ghost" onClick={onRefresh} disabled={!logs}>
          <RefreshCw size={14} /> Refresh
        </button>
      </div>
      {!logs ? (
        <Empty>Pick a container to view logs.</Empty>
      ) : (
        <Card title={`Logs · ${logs.container.name}`} subtitle={`last ${tail} lines`}>
          <pre className="max-h-[32rem] overflow-auto rounded-lg border border-ink-800 bg-ink-950 p-3 font-mono text-[11px] leading-relaxed text-slate-300 whitespace-pre-wrap">
            {logs.text}
          </pre>
        </Card>
      )}
    </div>
  );
}
