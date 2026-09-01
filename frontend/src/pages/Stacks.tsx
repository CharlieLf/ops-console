import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { AlertTriangle, RefreshCw, Search } from "lucide-react";
import { api } from "../api";
import { bytes } from "../format";
import { useAsync } from "../hooks";
import { Bar, Empty, ErrorBanner, Spinner } from "../components/ui";
import type { StackView } from "../types";

export default function Stacks() {
  const { data, error, loading, reload } = useAsync(() => api.stacks(), 10000);
  const [q, setQ] = useState("");

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

  if (loading && !data) return <Spinner label="Loading stacks" />;
  if (error && !data) return <ErrorBanner message={error} />;

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold text-slate-100">Stacks</h1>
          <p className="text-sm text-slate-400">
            {data?.stacks.length ?? 0} projects · click a card for details
          </p>
        </div>
        <div className="flex items-center gap-2">
          <div className="relative">
            <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-slate-500" />
            <input
              className="input w-48 pl-9"
              placeholder="Search stacks…"
              value={q}
              onChange={(e) => setQ(e.target.value)}
            />
          </div>
          <button className="btn btn-ghost" onClick={reload}>
            <RefreshCw size={14} />
          </button>
        </div>
      </header>

      {error && <ErrorBanner message={error} />}

      {stacks.length === 0 ? (
        <Empty>{q ? "No stacks match your search." : "No containers found."}</Empty>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {stacks.map((stack) => (
            <StackCard key={stack.name} stack={stack} />
          ))}
        </div>
      )}
    </div>
  );
}

function StackCard({ stack }: { stack: StackView }) {
  const degraded = stack.running < stack.total;
  const unhealthy = stack.containers.some((c) => c.health === "unhealthy");
  const path = stack.workingDir || stack.configFiles[0] || "—";

  return (
    <Link
      to={`/stacks/${encodeURIComponent(stack.name)}`}
      className="group block rounded-xl border border-ink-700 bg-ink-900/80 p-4 shadow-lg shadow-black/20 transition hover:border-sky-500/40 hover:bg-ink-850"
    >
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <h2 className="truncate text-sm font-semibold text-slate-100 group-hover:text-sky-200">
            {stack.name}
          </h2>
          <p className="mt-0.5 truncate font-mono text-[11px] text-slate-500">{path}</p>
        </div>
        <span
          className={`shrink-0 rounded-full border px-2 py-0.5 text-xs font-medium ${
            degraded
              ? "border-amber-500/40 bg-amber-500/10 text-amber-200"
              : "border-emerald-500/40 bg-emerald-500/10 text-emerald-200"
          }`}
        >
          {stack.running}/{stack.total}
        </span>
      </div>

      <div className="mt-4 space-y-2">
        {(stack.cpuPct ?? 0) > 0 && (
          <MetricRow label="CPU" value={`${stack.cpuPct!.toFixed(1)}%`} ratio={Math.min(stack.cpuPct! / 100, 1)} />
        )}
        {(stack.memUsage ?? 0) > 0 && (
          <MetricRow
            label="RAM"
            value={bytes(stack.memUsage)}
            ratio={Math.min((stack.memPct ?? 0) / 100, 1)}
          />
        )}
      </div>

      <div className="mt-3 flex flex-wrap gap-1.5">
        {unhealthy && (
          <span className="inline-flex items-center gap-1 rounded-full border border-rose-500/40 bg-rose-500/10 px-2 py-0.5 text-[10px] text-rose-200">
            <AlertTriangle size={10} /> unhealthy
          </span>
        )}
        {stack.networks.slice(0, 2).map((n) => (
          <span key={n} className="rounded-full border border-ink-600 bg-ink-800 px-2 py-0.5 font-mono text-[10px] text-slate-400">
            {n}
          </span>
        ))}
        {stack.containers.length > 0 && (
          <span className="text-[10px] text-slate-600">
            {stack.containers.map((c) => c.name).join(", ").slice(0, 40)}
            {stack.containers.length > 2 ? "…" : ""}
          </span>
        )}
      </div>
    </Link>
  );
}

function MetricRow({ label, value, ratio }: { label: string; value: string; ratio: number }) {
  return (
    <div>
      <div className="mb-0.5 flex justify-between text-[10px] text-slate-500">
        <span>{label}</span>
        <span className="font-mono text-slate-400">{value}</span>
      </div>
      <Bar ratio={ratio} tone={ratio >= 0.85 ? "bad" : ratio >= 0.7 ? "warn" : "good"} />
    </div>
  );
}
