import { useEffect, useMemo, useState } from "react";
import { AlertTriangle, Eye, History, Play, Save } from "lucide-react";
import { api } from "../api";
import { bytes, dateTime, duration, relative } from "../format";
import { useAsync } from "../hooks";
import { Card, Chip, Empty, ErrorBanner, Spinner, StatTile, Toggle } from "../components/ui";
import type { PruneConfig, PruneRun, PruneStatus, PruneTargets } from "../types";

const weekdays = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];

const targetCopy: { key: keyof PruneTargets; label: string; hint: string; danger?: boolean }[] = [
  { key: "buildCache", label: "Build cache", hint: "Layers left over from docker build. Safe, rebuilds are just slower." },
  { key: "danglingImages", label: "Dangling images", hint: "Untagged layers orphaned by a rebuild. Safe." },
  { key: "stoppedContainers", label: "Stopped containers", hint: "Exited containers past the minimum age." },
  { key: "networks", label: "Unused networks", hint: "Networks with nothing attached." },
  { key: "unusedImages", label: "Unused tagged images", hint: "Tagged images no container references. Re-pull needed after." , danger: true },
  { key: "volumes", label: "Unused volumes", hint: "Volumes no container references. This deletes data.", danger: true },
];

const timezones: string[] = (() => {
  const intl = Intl as unknown as { supportedValuesOf?: (key: string) => string[] };
  const list = intl.supportedValuesOf?.("timeZone");
  return list?.length ? list : ["UTC", "Asia/Jakarta", "Asia/Singapore", "Europe/London", "America/New_York"];
})();

export default function Prune() {
  const { data: status, error, loading, reload } = useAsync<PruneStatus>(() => api.pruneStatus(), 20000);
  const { data: historyData, reload: reloadHistory } = useAsync(() => api.pruneHistory(), 60000);

  const [draft, setDraft] = useState<PruneConfig | null>(null);
  const [busy, setBusy] = useState<"" | "save" | "preview" | "run">("");
  const [message, setMessage] = useState<{ tone: "ok" | "bad"; text: string } | null>(null);
  const [result, setResult] = useState<PruneRun | null>(null);

  useEffect(() => {
    if (status && !draft) setDraft(status.config);
  }, [status, draft]);

  const dirty = useMemo(
    () => Boolean(draft && status && JSON.stringify(draft) !== JSON.stringify(status.config)),
    [draft, status],
  );

  if (loading && !status) return <Spinner label="Loading prune settings" />;
  if (error && !status) return <ErrorBanner message={error} />;
  if (!status || !draft) return null;

  const patch = (changes: Partial<PruneConfig>) => setDraft({ ...draft, ...changes });
  const patchTarget = (key: keyof PruneTargets, value: boolean) =>
    setDraft({ ...draft, targets: { ...draft.targets, [key]: value } });

  const save = async () => {
    setBusy("save");
    setMessage(null);
    try {
      const res = await api.savePruneConfig(draft);
      setDraft(res.config);
      await reload();
      setMessage({ tone: "ok", text: "Schedule saved." });
    } catch (err) {
      setMessage({ tone: "bad", text: err instanceof Error ? err.message : String(err) });
    } finally {
      setBusy("");
    }
  };

  const execute = async (dryRun: boolean) => {
    if (!dryRun) {
      const destructive = draft.targets.volumes || draft.targets.unusedImages;
      const warning = destructive
        ? "This run can delete unused volumes and tagged images. Continue?"
        : "Run a prune now with these settings?";
      if (!window.confirm(warning)) return;
    }
    setBusy(dryRun ? "preview" : "run");
    setMessage(null);
    try {
      const res = await api.runPrune(dryRun, draft);
      setResult(res.run);
      await Promise.all([reload(), reloadHistory()]);
      setMessage({
        tone: "ok",
        text: dryRun
          ? `Preview: ${res.run.items.length} objects, ${bytes(res.run.reclaimed)} would be freed.`
          : `Pruned ${res.run.items.filter((i) => i.removed).length} objects, ${bytes(res.run.reclaimed)} freed.`,
      });
    } catch (err) {
      setMessage({ tone: "bad", text: err instanceof Error ? err.message : String(err) });
    } finally {
      setBusy("");
    }
  };

  const history = historyData?.history ?? [];

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold text-slate-100">Auto-prune</h1>
          <p className="text-sm text-slate-400">
            Scheduled Docker cleanup with age limits, keep patterns and a dry-run preview
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <button className="btn btn-ghost" disabled={busy !== ""} onClick={() => execute(true)}>
            <Eye size={14} />
            {busy === "preview" ? "Previewing" : "Preview"}
          </button>
          <button className="btn btn-danger" disabled={busy !== ""} onClick={() => execute(false)}>
            <Play size={14} />
            {busy === "run" ? "Pruning" : "Run now"}
          </button>
          <button className="btn btn-primary" disabled={!dirty || busy !== ""} onClick={save}>
            <Save size={14} />
            {busy === "save" ? "Saving" : dirty ? "Save changes" : "Saved"}
          </button>
        </div>
      </header>

      {message && (
        <div
          className={`card px-4 py-3 text-sm ${
            message.tone === "ok"
              ? "border-emerald-500/40 bg-emerald-500/10 text-emerald-200"
              : "border-rose-500/40 bg-rose-500/10 text-rose-200"
          }`}
        >
          {message.text}
        </div>
      )}

      <div className="grid gap-4 sm:grid-cols-3">
        <StatTile
          label="Status"
          value={status.enabled ? (status.running ? "running" : "armed") : "off"}
          tone={status.enabled ? "good" : "warn"}
          hint={status.config.dryRun ? "scheduled runs are preview only" : "scheduled runs delete"}
        />
        <StatTile
          label="Next run"
          value={status.enabled ? relative(status.nextRun) : "-"}
          hint={status.enabled ? dateTime(status.nextRun) : "schedule disabled"}
        />
        <StatTile
          label="Last run"
          value={relative(status.lastRun)}
          hint={status.lastResult ? `${bytes(status.lastResult.reclaimed)} freed` : "no runs yet"}
        />
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card title="Schedule" subtitle="When the cleanup fires">
          <div className="space-y-4">
            <Toggle
              checked={draft.enabled}
              onChange={(v) => patch({ enabled: v })}
              label="Run automatically"
              hint="Off means cleanup only happens when you press Run now."
            />
            <div>
              <span className="label">Frequency</span>
              <div className="flex gap-1 rounded-lg border border-ink-700 p-0.5">
                {(["interval", "daily", "weekly"] as const).map((mode) => (
                  <button
                    key={mode}
                    onClick={() => patch({ mode })}
                    className={`flex-1 rounded-md px-3 py-1.5 text-xs capitalize transition-colors ${
                      draft.mode === mode ? "bg-ink-700 text-slate-100" : "text-slate-400 hover:text-slate-200"
                    }`}
                  >
                    {mode}
                  </button>
                ))}
              </div>
            </div>

            {draft.mode === "interval" ? (
              <div>
                <label className="label" htmlFor="interval">Every N hours</label>
                <input
                  id="interval"
                  type="number"
                  min={1}
                  max={720}
                  className="input"
                  value={draft.intervalHours}
                  onChange={(e) => patch({ intervalHours: Number(e.target.value) })}
                />
              </div>
            ) : (
              <div className="grid gap-3 sm:grid-cols-2">
                <div>
                  <label className="label" htmlFor="time">Time of day</label>
                  <input
                    id="time"
                    type="time"
                    className="input"
                    value={draft.timeOfDay}
                    onChange={(e) => patch({ timeOfDay: e.target.value })}
                  />
                </div>
                {draft.mode === "weekly" && (
                  <div>
                    <label className="label" htmlFor="weekday">Day</label>
                    <select
                      id="weekday"
                      className="input"
                      value={draft.weekday}
                      onChange={(e) => patch({ weekday: Number(e.target.value) })}
                    >
                      {weekdays.map((d, i) => (
                        <option key={d} value={i}>{d}</option>
                      ))}
                    </select>
                  </div>
                )}
              </div>
            )}

            <div>
              <label className="label" htmlFor="tz">Timezone</label>
              <select
                id="tz"
                className="input"
                value={draft.timezone}
                onChange={(e) => patch({ timezone: e.target.value })}
              >
                {timezones.includes(draft.timezone) ? null : (
                  <option value={draft.timezone}>{draft.timezone}</option>
                )}
                {timezones.map((tz) => (
                  <option key={tz} value={tz}>{tz}</option>
                ))}
              </select>
            </div>

            <Toggle
              checked={draft.dryRun}
              onChange={(v) => patch({ dryRun: v })}
              label="Scheduled runs are preview only"
              hint="Useful for a week of observation before letting it delete anything."
            />
          </div>
        </Card>

        <Card title="What gets removed" subtitle="Each target is evaluated independently">
          <div className="space-y-2">
            {targetCopy.map((t) => (
              <Toggle
                key={t.key}
                checked={draft.targets[t.key]}
                onChange={(v) => patchTarget(t.key, v)}
                label={t.danger ? `${t.label} (destructive)` : t.label}
                hint={t.hint}
              />
            ))}
          </div>
        </Card>
      </div>

      <Card title="Safety" subtitle="Guards applied to every run, manual or scheduled">
        <div className="grid gap-4 sm:grid-cols-2">
          <div>
            <label className="label" htmlFor="minage">Minimum age (hours)</label>
            <input
              id="minage"
              type="number"
              min={0}
              className="input"
              value={draft.minAgeHours}
              onChange={(e) => patch({ minAgeHours: Number(e.target.value) })}
            />
            <p className="mt-1 text-xs text-slate-500">
              Objects created within this window are never touched, so a fresh build is safe.
            </p>
          </div>
          <div>
            <label className="label" htmlFor="keep">Keep patterns (one per line)</label>
            <textarea
              id="keep"
              rows={4}
              className="input font-mono text-xs"
              value={draft.keepPatterns.join("\n")}
              onChange={(e) =>
                patch({ keepPatterns: e.target.value.split("\n").map((s) => s.trim()).filter(Boolean) })
              }
              placeholder={"proxy\nmonitoring_*\npostgres:*"}
            />
            <p className="mt-1 text-xs text-slate-500">
              Matched against image refs, volume, container and network names. Globs allowed; plain text
              matches as a substring. The label <code className="text-slate-400">ops-console.keep=true</code>{" "}
              protects an object too.
            </p>
          </div>
        </div>
      </Card>

      {result && <RunDetail run={result} title="Last run from this page" />}

      <Card
        title={
          <span className="flex items-center gap-2">
            <History size={14} /> History
          </span>
        }
        subtitle="The last 60 runs, scheduled and manual"
      >
        {history.length === 0 ? (
          <Empty>No prune runs recorded yet.</Empty>
        ) : (
          <ul className="divide-y divide-ink-800">
            {history.map((run) => (
              <li key={run.id} className="flex flex-wrap items-center gap-3 py-2.5 text-sm">
                <Chip tone={run.dryRun ? "muted" : "accent"}>{run.dryRun ? "preview" : run.trigger}</Chip>
                <span className="text-slate-300">{dateTime(run.startedAt)}</span>
                <span className="text-slate-500">{run.items.filter((i) => i.removed || run.dryRun).length} objects</span>
                <span className="text-slate-500">{duration(run.startedAt, run.finishedAt)}</span>
                {run.errors.length > 0 && (
                  <span className="flex items-center gap-1 text-xs text-amber-300">
                    <AlertTriangle size={12} /> {run.errors.length} errors
                  </span>
                )}
                <span className="ml-auto font-mono text-slate-200">{bytes(run.reclaimed)}</span>
              </li>
            ))}
          </ul>
        )}
      </Card>
    </div>
  );
}

function RunDetail({ run, title }: { run: PruneRun; title: string }) {
  return (
    <Card
      title={title}
      subtitle={`${run.dryRun ? "Preview" : "Executed"} · ${dateTime(run.startedAt)} · ${bytes(run.reclaimed)} ${
        run.dryRun ? "would be freed" : "freed"
      }`}
    >
      {run.items.length === 0 ? (
        <Empty>Nothing matched. There was no garbage to collect.</Empty>
      ) : (
        <ul className="max-h-80 space-y-1 overflow-y-auto">
          {run.items.map((item, i) => (
            <li
              key={`${item.kind}-${item.ref}-${i}`}
              className="flex flex-wrap items-center gap-2 rounded-md bg-ink-850/50 px-3 py-1.5 text-xs"
            >
              <Chip>{item.kind}</Chip>
              <span className="font-mono text-slate-200">{item.ref}</span>
              {item.reason && <span className="text-slate-500">{item.reason}</span>}
              <span className="ml-auto font-mono text-slate-400">{bytes(item.size)}</span>
            </li>
          ))}
        </ul>
      )}
      {run.errors.length > 0 && (
        <ul className="mt-3 space-y-1 border-t border-ink-800 pt-3 text-xs text-amber-300">
          {run.errors.map((e, i) => (
            <li key={i}>{e}</li>
          ))}
        </ul>
      )}
    </Card>
  );
}
