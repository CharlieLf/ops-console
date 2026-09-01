import { useMemo } from "react";
import { RefreshCw } from "lucide-react";
import { api } from "../api";
import { bytes } from "../format";
import { useAsync } from "../hooks";
import { Bar, Card, Empty, ErrorBanner, Spinner, StatTile } from "../components/ui";
import type { ContainerStat, MetricsSnapshot } from "../types";

export default function Metrics() {
  const { data, error, loading, reload } = useAsync<MetricsSnapshot>(() => api.metrics(), 5000);

  if (loading && !data) return <Spinner label="Sampling host and containers" />;
  if (error && !data) return <ErrorBanner message={error} />;
  if (!data) return null;

  const { host, history, containers, disk } = data;

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold text-slate-100">Metrics</h1>
          <p className="text-sm text-slate-400">Live host + container usage · ~15s samples</p>
        </div>
        <button className="btn btn-ghost" onClick={reload}>
          <RefreshCw size={14} />
          Refresh
        </button>
      </header>

      {error && <ErrorBanner message={error} />}

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <StatTile
          label="CPU"
          value={`${host.cpuPct.toFixed(1)}%`}
          tone={host.cpuPct >= 90 ? "bad" : host.cpuPct >= 70 ? "warn" : "good"}
        />
        <StatTile
          label="Memory"
          value={`${host.memPct.toFixed(1)}%`}
          tone={host.memPct >= 90 ? "bad" : host.memPct >= 80 ? "warn" : "good"}
          hint={`${bytes(host.memUsed)} / ${bytes(host.memTotal)}`}
        />
        <StatTile
          label="Disk"
          value={`${Math.round(disk.usedRatio * 100)}%`}
          tone={disk.usedRatio >= 0.9 ? "bad" : disk.usedRatio >= 0.8 ? "warn" : "good"}
          hint={`${bytes(disk.free)} free`}
        />
        <StatTile label="Containers" value={containers.length} hint="running with stats" />
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card title="CPU history" subtitle="last ~30 minutes">
          <Sparkline values={history.map((h) => h.cpuPct)} max={100} color="rgb(56 189 248)" />
        </Card>
        <Card title="Memory history" subtitle="last ~30 minutes">
          <Sparkline values={history.map((h) => h.memPct)} max={100} color="rgb(52 211 153)" />
        </Card>
      </div>

      <Card title="Containers" subtitle="sorted by memory">
        {containers.length === 0 ? (
          <Empty>No running containers.</Empty>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead className="text-xs uppercase text-slate-500">
                <tr>
                  <th className="pb-2 pr-3 font-medium">Name</th>
                  <th className="pb-2 pr-3 font-medium">CPU</th>
                  <th className="pb-2 pr-3 font-medium">Memory</th>
                  <th className="pb-2 font-medium">Mem %</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-ink-800">
                {containers.map((c) => (
                  <ContainerRow key={c.id} c={c} />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>
    </div>
  );
}

function ContainerRow({ c }: { c: ContainerStat }) {
  return (
    <tr>
      <td className="py-2.5 pr-3 font-medium text-slate-200">{c.name}</td>
      <td className="py-2.5 pr-3 font-mono text-slate-300">{c.cpuPct.toFixed(1)}%</td>
      <td className="py-2.5 pr-3 font-mono text-slate-300">
        {bytes(c.memUsage)}
        {c.memLimit > 0 && c.memLimit < 1e14 ? (
          <span className="text-slate-500"> / {bytes(c.memLimit)}</span>
        ) : null}
      </td>
      <td className="py-2.5 min-w-[8rem]">
        <div className="flex items-center gap-2">
          <Bar ratio={Math.min(c.memPct / 100, 1)} />
          <span className="w-12 shrink-0 text-right font-mono text-xs text-slate-400">
            {c.memPct.toFixed(0)}%
          </span>
        </div>
      </td>
    </tr>
  );
}

function Sparkline({
  values,
  max,
  color,
}: {
  values: number[];
  max: number;
  color: string;
}) {
  const points = useMemo(() => {
    if (values.length < 2) return "";
    const w = 100;
    const h = 36;
    return values
      .map((v, i) => {
        const x = (i / (values.length - 1)) * w;
        const y = h - (Math.min(Math.max(v, 0), max) / max) * h;
        return `${x},${y}`;
      })
      .join(" ");
  }, [values, max]);

  if (values.length < 2) {
    return <p className="text-sm text-slate-500">Collecting samples…</p>;
  }

  const latest = values[values.length - 1] ?? 0;
  return (
    <div>
      <svg viewBox="0 0 100 36" className="h-24 w-full" preserveAspectRatio="none">
        <polyline fill="none" stroke={color} strokeWidth="1.5" points={points} vectorEffect="non-scaling-stroke" />
      </svg>
      <p className="mt-1 text-right text-xs text-slate-500">now {latest.toFixed(1)}%</p>
    </div>
  );
}
