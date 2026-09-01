import { useMemo, useState } from "react";
import { RefreshCw } from "lucide-react";
import { api } from "../api";
import { useAsync } from "../hooks";
import { Card, Empty, ErrorBanner, SeverityBadge, Spinner } from "../components/ui";
import type { Severity } from "../types";

const filters: { key: Severity | "all"; label: string }[] = [
  { key: "all", label: "All" },
  { key: "high", label: "High" },
  { key: "warn", label: "Warnings" },
  { key: "info", label: "Info" },
];

export default function Checks() {
  const { data, error, loading, reload } = useAsync(() => api.checks(), 30000);
  const [filter, setFilter] = useState<Severity | "all">("all");

  const findings = useMemo(() => {
    const all = data?.findings ?? [];
    return filter === "all" ? all : all.filter((f) => f.severity === filter);
  }, [data, filter]);

  if (loading && !data) return <Spinner label="Evaluating configuration" />;
  if (error && !data) return <ErrorBanner message={error} />;

  const summary = data?.summary ?? { high: 0, warn: 0, info: 0 };

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold text-slate-100">Checks</h1>
          <p className="text-sm text-slate-400">
            {summary.high} high · {summary.warn} warnings · {summary.info} informational
          </p>
        </div>
        <div className="flex items-center gap-2">
          <div className="flex rounded-lg border border-ink-700 p-0.5">
            {filters.map((f) => (
              <button
                key={f.key}
                onClick={() => setFilter(f.key)}
                className={`rounded-md px-3 py-1.5 text-xs transition-colors ${
                  filter === f.key ? "bg-ink-700 text-slate-100" : "text-slate-400 hover:text-slate-200"
                }`}
              >
                {f.label}
              </button>
            ))}
          </div>
          <button className="btn btn-ghost" onClick={reload}>
            <RefreshCw size={14} />
            Rescan
          </button>
        </div>
      </header>

      <Card>
        {findings.length === 0 ? (
          <Empty>No findings in this category.</Empty>
        ) : (
          <ul className="divide-y divide-ink-800">
            {findings.map((f) => (
              <li key={f.id} className="py-3">
                <div className="flex flex-wrap items-center gap-2">
                  <SeverityBadge severity={f.severity} />
                  <span className="text-sm font-medium text-slate-100">{f.title}</span>
                  <span className="font-mono text-xs text-slate-500">{f.target}</span>
                  <span className="ml-auto text-[11px] uppercase tracking-wide text-slate-600">
                    {f.category}
                  </span>
                </div>
                <p className="mt-1 text-sm text-slate-400">{f.detail}</p>
                {f.hint && <p className="mt-1 text-xs text-sky-300/80">{f.hint}</p>}
              </li>
            ))}
          </ul>
        )}
      </Card>
    </div>
  );
}
