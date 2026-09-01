import { useMemo, useState, type MouseEvent } from "react";
import { Link } from "react-router-dom";
import { AlertTriangle, Play, Power, RefreshCw, Search } from "lucide-react";
import { api } from "../api";
import { bytes } from "../format";
import { useAsync } from "../hooks";
import { Bar, Empty, ErrorBanner, Spinner, StateBadge } from "../components/ui";
import type { ContainerView, StackView } from "../types";

export default function Stacks() {
  const { data, error, loading, reload } = useAsync(() => api.stacks(), 10000);
  const [q, setQ] = useState("");
  const [busy, setBusy] = useState<string | null>(null);
  const [msg, setMsg] = useState<string | null>(null);

  const stacks = useMemo(() => {
    const all = data?.stacks ?? [];
    const needle = q.trim().toLowerCase();
    if (!needle) return all;
    return all.filter(
      (s) =>
        s.name.toLowerCase().includes(needle) ||
        s.workingDir.toLowerCase().includes(needle) ||
        s.containers.some((c) => c.name.toLowerCase().includes(needle)),
    );
  }, [data, q]);

  async function startContainer(e: MouseEvent, id: string) {
    e.preventDefault();
    e.stopPropagation();
    setBusy(id);
    setMsg(null);
    try {
      await api.containerAction(id, "start");
      await reload();
    } catch (err) {
      setMsg(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(null);
    }
  }

  async function startStack(e: MouseEvent, name: string) {
    e.preventDefault();
    e.stopPropagation();
    setBusy(`stack:${name}`);
    setMsg(null);
    try {
      const res = await api.stackAction(name, "up");
      setMsg(res.output || `Started ${name}`);
      await reload();
    } catch (err) {
      setMsg(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(null);
    }
  }

  if (loading && !data) return <Spinner label="Loading stacks" />;
  if (error && !data) return <ErrorBanner message={error} />;

  return (
    <div className="space-y-6 text-base">
      <header className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold text-slate-100">Stacks</h1>
          <p className="text-base text-slate-400">
            {data?.stacks.length ?? 0} projects · running and offline stacks
          </p>
        </div>
        <div className="flex items-center gap-2">
          <div className="relative">
            <Search size={16} className="absolute left-3 top-1/2 -translate-y-1/2 text-slate-500" />
            <input
              className="input w-56 pl-9 text-base"
              placeholder="Search stacks…"
              value={q}
              onChange={(e) => setQ(e.target.value)}
            />
          </div>
          <button className="btn btn-ghost" onClick={reload}>
            <RefreshCw size={16} />
          </button>
        </div>
      </header>

      {msg && (
        <div className="rounded-lg border border-ink-700 bg-ink-900/80 px-4 py-3 text-base text-slate-300 whitespace-pre-wrap">
          {msg}
          <button className="ml-3 text-sm text-slate-500 underline" onClick={() => setMsg(null)}>
            dismiss
          </button>
        </div>
      )}
      {error && <ErrorBanner message={error} />}

      {stacks.length === 0 ? (
        <Empty>{q ? "No stacks match your search." : "No stacks found."}</Empty>
      ) : (
        <div className="grid gap-5 sm:grid-cols-2 xl:grid-cols-3">
          {stacks.map((stack) => (
            <StackCard
              key={stack.name}
              stack={stack}
              busy={busy}
              onStartContainer={startContainer}
              onStartStack={startStack}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function StackCard({
  stack,
  busy,
  onStartContainer,
  onStartStack,
}: {
  stack: StackView;
  busy: string | null;
  onStartContainer: (e: MouseEvent, id: string) => void;
  onStartStack: (e: MouseEvent, name: string) => void;
}) {
  const offline = !stack.deployed || stack.running === 0;
  const degraded = stack.deployed && stack.running < stack.total;
  const unhealthy = stack.containers.some((c) => c.health === "unhealthy");
  const path = stack.workingDir || stack.configFiles[0] || "—";
  const stopped = stack.containers.filter((c) => c.state !== "running");

  return (
    <Link
      to={`/stacks/${encodeURIComponent(stack.name)}`}
      className={`group block rounded-xl border p-5 shadow-lg shadow-black/20 transition hover:bg-ink-850 ${
        offline
          ? "border-slate-600/60 bg-ink-900/50 hover:border-slate-500"
          : "border-ink-700 bg-ink-900/80 hover:border-sky-500/40"
      }`}
    >
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <h2 className="truncate text-lg font-semibold text-slate-100 group-hover:text-sky-200">
            {stack.name}
          </h2>
          <p className="mt-1 truncate font-mono text-sm text-slate-500">{path}</p>
        </div>
        <div className="flex shrink-0 flex-col items-end gap-1.5">
          {offline ? (
            <span className="rounded-full border border-slate-500/50 bg-slate-500/10 px-2.5 py-0.5 text-sm font-medium text-slate-300">
              offline
            </span>
          ) : (
            <span
              className={`rounded-full border px-2.5 py-0.5 text-sm font-medium ${
                degraded
                  ? "border-amber-500/40 bg-amber-500/10 text-amber-200"
                  : "border-emerald-500/40 bg-emerald-500/10 text-emerald-200"
              }`}
            >
              {stack.running}/{stack.total} up
            </span>
          )}
          {offline && stack.name !== "(standalone)" && (
            <button
              className="btn btn-primary !px-2.5 !py-1 text-sm"
              disabled={!!busy}
              onClick={(e) => onStartStack(e, stack.name)}
            >
              <Power size={14} />
              {busy === `stack:${stack.name}` ? "Starting…" : "Start stack"}
            </button>
          )}
        </div>
      </div>

      {!offline && (stack.cpuPct ?? 0) > 0 && (
        <div className="mt-4 space-y-2">
          <MetricRow label="CPU" value={`${stack.cpuPct!.toFixed(1)}%`} ratio={Math.min(stack.cpuPct! / 100, 1)} />
          {(stack.memUsage ?? 0) > 0 && (
            <MetricRow
              label="RAM"
              value={bytes(stack.memUsage)}
              ratio={Math.min((stack.memPct ?? 0) / 100, 1)}
            />
          )}
        </div>
      )}

      <div className="mt-4 space-y-2">
        <p className="text-sm font-medium text-slate-400">Containers</p>
        {stack.containers.length === 0 ? (
          <p className="text-sm text-slate-500">No containers — use Start stack to bring this project up.</p>
        ) : (
          <ul className="space-y-1.5">
            {stack.containers.map((c) => (
              <ContainerLine
                key={c.id}
                container={c}
                busy={busy === c.id}
                onStart={onStartContainer}
              />
            ))}
          </ul>
        )}
        {stopped.length > 0 && stack.deployed && (
          <p className="text-sm text-amber-300/90">{stopped.length} stopped — click ▶ to start</p>
        )}
      </div>

      <div className="mt-3 flex flex-wrap gap-2">
        {unhealthy && (
          <span className="inline-flex items-center gap-1 rounded-full border border-rose-500/40 bg-rose-500/10 px-2.5 py-0.5 text-sm text-rose-200">
            <AlertTriangle size={12} /> unhealthy
          </span>
        )}
        {stack.networks.slice(0, 2).map((n) => (
          <span key={n} className="rounded-full border border-ink-600 bg-ink-800 px-2.5 py-0.5 font-mono text-sm text-slate-400">
            {n}
          </span>
        ))}
      </div>
    </Link>
  );
}

function ContainerLine({
  container,
  busy,
  onStart,
}: {
  container: ContainerView;
  busy: boolean;
  onStart: (e: MouseEvent, id: string) => void;
}) {
  const running = container.state === "running";
  return (
    <li
      className={`flex items-center gap-2 rounded-lg border px-2.5 py-2 text-sm ${
        running ? "border-ink-700 bg-ink-850/40" : "border-amber-500/30 bg-amber-500/5"
      }`}
      onClick={(e) => e.stopPropagation()}
    >
      <StateBadge state={container.state} health={container.health} />
      <span className="min-w-0 flex-1 truncate font-medium text-slate-200">{container.name}</span>
      {!running && (
        <button
          className="btn btn-primary !px-2 !py-1"
          title="Start container"
          disabled={busy}
          onClick={(e) => onStart(e, container.id)}
        >
          <Play size={14} />
        </button>
      )}
    </li>
  );
}

function MetricRow({ label, value, ratio }: { label: string; value: string; ratio: number }) {
  return (
    <div>
      <div className="mb-1 flex justify-between text-sm text-slate-500">
        <span>{label}</span>
        <span className="font-mono text-slate-400">{value}</span>
      </div>
      <Bar ratio={ratio} tone={ratio >= 0.85 ? "bad" : ratio >= 0.7 ? "warn" : "good"} />
    </div>
  );
}
