import { useState } from "react";
import { Lock, RefreshCw } from "lucide-react";
import { api } from "../api";
import { bytes, relative } from "../format";
import { useAsync } from "../hooks";
import { Card, Empty, ErrorBanner, Spinner, StatTile } from "../components/ui";
import type { ResourceRow } from "../types";

type Tab = "images" | "volumes" | "networks";

export default function Resources() {
  const { data, error, loading, reload } = useAsync(() => api.resources(), 30000);
  const [tab, setTab] = useState<Tab>("images");
  const [unusedOnly, setUnusedOnly] = useState(false);

  if (loading && !data) return <Spinner label="Measuring images, volumes and networks" />;
  if (error && !data) return <ErrorBanner message={error} />;
  if (!data) return null;

  const rows = data[tab].filter((r) => !unusedOnly || !r.inUse);
  const totals = data.totals;

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold text-slate-100">Resources</h1>
          <p className="text-sm text-slate-400">Everything the daemon is storing, and what is idle</p>
        </div>
        <button className="btn btn-ghost" onClick={reload}>
          <RefreshCw size={14} />
          Refresh
        </button>
      </header>

      <div className="grid gap-4 sm:grid-cols-3">
        <StatTile label="Images" value={bytes(totals.imagesSize)} hint={`${bytes(totals.imagesReclaimable)} unused`} />
        <StatTile label="Volumes" value={bytes(totals.volumesSize)} hint={`${bytes(totals.volumesReclaimable)} unused`} />
        <StatTile
          label="Build cache"
          value={bytes(totals.buildCacheSize)}
          hint={`${bytes(totals.buildCacheReclaimable)} reclaimable`}
        />
      </div>

      <Card
        actions={
          <label className="flex items-center gap-2 text-xs text-slate-400">
            <input
              type="checkbox"
              checked={unusedOnly}
              onChange={(e) => setUnusedOnly(e.target.checked)}
              className="h-3.5 w-3.5 accent-sky-500"
            />
            unused only
          </label>
        }
        title={
          <div className="flex gap-1">
            {(["images", "volumes", "networks"] as Tab[]).map((t) => (
              <button
                key={t}
                onClick={() => setTab(t)}
                className={`rounded-md px-3 py-1.5 text-xs capitalize transition-colors ${
                  tab === t ? "bg-ink-700 text-slate-100" : "text-slate-400 hover:text-slate-200"
                }`}
              >
                {t} ({data[t].length})
              </button>
            ))}
          </div>
        }
      >
        {rows.length === 0 ? (
          <Empty>Nothing to show.</Empty>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead className="text-xs uppercase tracking-wide text-slate-500">
                <tr>
                  <th className="pb-2 font-medium">Name</th>
                  <th className="pb-2 font-medium">Detail</th>
                  <th className="pb-2 font-medium">Created</th>
                  <th className="pb-2 text-right font-medium">Size</th>
                  <th className="pb-2 text-right font-medium">State</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-ink-800">
                {rows.map((row) => (
                  <Row key={`${row.name}-${row.ref}`} row={row} showSize={tab !== "networks"} />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>
    </div>
  );
}

function Row({ row, showSize }: { row: ResourceRow; showSize: boolean }) {
  return (
    <tr>
      <td className="max-w-xs truncate py-2 pr-3 font-mono text-xs text-slate-200">{row.name}</td>
      <td className="max-w-xs truncate py-2 pr-3 text-xs text-slate-500">
        {row.ref}
        {row.extra ? ` · ${row.extra}` : ""}
      </td>
      <td className="whitespace-nowrap py-2 pr-3 text-xs text-slate-500">{relative(row.createdAt)}</td>
      <td className="py-2 pr-3 text-right text-xs text-slate-300">{showSize ? bytes(row.size) : "-"}</td>
      <td className="py-2 text-right">
        <span className="inline-flex items-center gap-1.5">
          {row.protected && <Lock size={11} className="text-sky-300" />}
          <span
            className={`chip ${
              row.inUse
                ? "border-emerald-500/40 bg-emerald-500/10 text-emerald-200"
                : "border-amber-500/40 bg-amber-500/10 text-amber-200"
            }`}
          >
            {row.inUse ? "in use" : "idle"}
          </span>
        </span>
      </td>
    </tr>
  );
}
