import { Link } from "react-router-dom";
import { RefreshCw } from "lucide-react";
import { api } from "../api";
import { bytes, dateTime, relative } from "../format";
import { useAsync } from "../hooks";
import { Bar, Card, Chip, Empty, ErrorBanner, SeverityBadge, Spinner, StatTile } from "../components/ui";
import type { Overview } from "../types";

export default function Dashboard() {
  const { data, error, loading, reload } = useAsync<Overview>(() => api.overview(), 15000);
  const metrics = useAsync(() => api.metrics(), 10000);
  const activity = useAsync(() => api.activity(15), 10000);
  const alerts = useAsync(() => api.alerts(), 10000);

  if (loading && !data) return <Spinner label="Reading the Docker daemon" />;
  if (error && !data) return <ErrorBanner message={error} />;
  if (!data) return null;

  const { host, disk, counts, storage, checks, prune } = data;
  const reclaimable =
    storage.imagesReclaimable + storage.volumesReclaimable + storage.buildCacheReclaimable;

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold text-slate-100">{host.name || "Docker host"}</h1>
          <p className="text-sm text-slate-400">
            Docker {host.dockerVersion} · {host.cpus} vCPU · {bytes(host.memTotal)} RAM · {host.arch}
          </p>
        </div>
        <button className="btn btn-ghost" onClick={reload}>
          <RefreshCw size={14} />
          Refresh
        </button>
      </header>

      {error && <ErrorBanner message={error} />}

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <StatTile
          label="Stacks"
          value={counts.stacks}
          hint={`${counts.containersRunning} running · ${counts.containersStopped} stopped`}
        />
        <StatTile
          label="Disk used"
          value={`${Math.round(disk.usedRatio * 100)}%`}
          tone={disk.usedRatio >= 0.9 ? "bad" : disk.usedRatio >= 0.8 ? "warn" : "good"}
          hint={`${bytes(disk.free)} free of ${bytes(disk.total)}`}
        />
        <StatTile
          label="Reclaimable"
          value={bytes(reclaimable)}
          tone={reclaimable > 2 * 1024 ** 3 ? "warn" : "default"}
          hint="images, volumes and build cache"
        />
        <StatTile
          label="Findings"
          value={checks.high + checks.warn}
          tone={checks.high > 0 ? "bad" : checks.warn > 0 ? "warn" : "good"}
          hint={`${checks.high} high · ${checks.warn} warn · ${checks.info} info`}
        />
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card title="Live alerts" subtitle="CPU, memory, health (Beszel-style)">
          {(alerts.data?.alerts.length ?? 0) === 0 ? (
            <Empty>No active alerts.</Empty>
          ) : (
            <ul className="divide-y divide-ink-800">
              {alerts.data!.alerts.slice(0, 8).map((a) => (
                <li key={a.id} className="flex items-center gap-2 py-2 text-sm">
                  <SeverityBadge severity={a.severity === "high" ? "high" : a.severity === "warn" ? "warn" : "info"} />
                  <span className="text-slate-200">{a.title}</span>
                  <Chip tone="muted">{a.target}</Chip>
                </li>
              ))}
            </ul>
          )}
        </Card>

        <Card title="Activity" subtitle="Recent container events (Portainer-style)">
          {(activity.data?.events.length ?? 0) === 0 ? (
            <Empty>No recent events.</Empty>
          ) : (
            <ul className="divide-y divide-ink-800">
              {activity.data!.events.slice(0, 10).map((ev, i) => (
                <li key={i} className="flex flex-wrap items-center gap-2 py-2 text-xs">
                  <Chip tone={ev.action === "die" || ev.action === "kill" ? "danger" : "accent"}>
                    {ev.action}
                  </Chip>
                  <span className="font-medium text-slate-200">{ev.name}</span>
                  <span className="text-slate-500">{relative(ev.time)}</span>
                </li>
              ))}
            </ul>
          )}
        </Card>
      </div>

      <Card title="Top consumers" subtitle="By memory · live from docker stats">
        {(metrics.data?.containers.length ?? 0) === 0 ? (
          <Empty>Collecting stats…</Empty>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead className="text-xs uppercase text-slate-500">
                <tr>
                  <th className="pb-2 font-medium">Container</th>
                  <th className="pb-2 font-medium">CPU</th>
                  <th className="pb-2 font-medium">Memory</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-ink-800">
                {metrics.data!.containers.slice(0, 8).map((c) => (
                  <tr key={c.id}>
                    <td className="py-2 pr-3 text-slate-200">{c.name}</td>
                    <td className="py-2 pr-3 font-mono text-slate-400">{c.cpuPct.toFixed(1)}%</td>
                    <td className="py-2 font-mono text-slate-400">
                      {bytes(c.memUsage)}
                      {c.memLimit > 0 && c.memLimit < 1e14 ? ` (${c.memPct.toFixed(0)}%)` : ""}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card title="Storage" subtitle="What Docker is holding on disk">
          <div className="space-y-4">
            <div>
              <div className="mb-1 flex justify-between text-sm">
                <span className="text-slate-300">Filesystem</span>
                <span className="text-slate-400">
                  {bytes(disk.used)} / {bytes(disk.total)}
                </span>
              </div>
              <Bar ratio={disk.usedRatio} />
            </div>
            <dl className="grid grid-cols-2 gap-3 text-sm">
              <Row label="Images" value={bytes(storage.imagesSize)} sub={`${bytes(storage.imagesReclaimable)} unused`} />
              <Row label="Volumes" value={bytes(storage.volumesSize)} sub={`${bytes(storage.volumesReclaimable)} unused`} />
              <Row label="Build cache" value={bytes(storage.buildCacheSize)} sub={`${bytes(storage.buildCacheReclaimable)} reclaimable`} />
              <Row label="Objects" value={`${counts.images} images`} sub={`${counts.volumes} volumes · ${counts.networks} networks`} />
            </dl>
          </div>
        </Card>

        <Card
          title="Auto-prune"
          subtitle={prune.enabled ? "Scheduled cleanup is on" : "Scheduled cleanup is off"}
          actions={
            <Link className="btn btn-ghost" to="/prune">
              Configure
            </Link>
          }
        >
          <dl className="grid grid-cols-2 gap-3 text-sm">
            <Row
              label="Next run"
              value={prune.enabled ? relative(prune.nextRun) : "disabled"}
              sub={prune.enabled ? dateTime(prune.nextRun) : "enable it to schedule"}
            />
            <Row label="Last run" value={relative(prune.lastRun)} sub={dateTime(prune.lastRun)} />
            <Row
              label="Last result"
              value={prune.lastResult ? bytes(prune.lastResult.reclaimed) : "-"}
              sub={
                prune.lastResult
                  ? `${prune.lastResult.items.length} objects · ${prune.lastResult.trigger}`
                  : "no runs yet"
              }
            />
            <Row
              label="Schedule"
              value={describeSchedule(prune)}
              sub={`keeps anything newer than ${prune.config.minAgeHours}h`}
            />
          </dl>
        </Card>
      </div>

      <Card
        title="Top findings"
        subtitle="Highest severity issues detected in the current configuration"
        actions={
          <Link className="btn btn-ghost" to="/checks">
            All checks
          </Link>
        }
      >
        {data.topFindings.length === 0 ? (
          <Empty>Nothing flagged. The stacks look healthy.</Empty>
        ) : (
          <ul className="divide-y divide-ink-800">
            {data.topFindings.map((f) => (
              <li key={f.id} className="flex flex-wrap items-center gap-3 py-2.5 text-sm">
                <SeverityBadge severity={f.severity} />
                <span className="font-medium text-slate-200">{f.title}</span>
                <span className="font-mono text-xs text-slate-500">{f.target}</span>
              </li>
            ))}
          </ul>
        )}
      </Card>
    </div>
  );
}

function Row({ label, value, sub }: { label: string; value: string | number; sub?: string }) {
  return (
    <div>
      <dt className="text-xs uppercase tracking-wide text-slate-500">{label}</dt>
      <dd className="text-slate-100">{value}</dd>
      {sub && <dd className="text-xs text-slate-500">{sub}</dd>}
    </div>
  );
}

function describeSchedule(prune: Overview["prune"]): string {
  const { mode, intervalHours, timeOfDay, weekday } = prune.config;
  const days = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];
  if (mode === "interval") return `every ${intervalHours}h`;
  if (mode === "weekly") return `${days[weekday]} ${timeOfDay}`;
  return `daily ${timeOfDay}`;
}
