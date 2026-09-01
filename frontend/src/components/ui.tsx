import type { ReactNode } from "react";
import { AlertTriangle, Info, Loader2, ShieldAlert } from "lucide-react";
import type { Severity } from "../types";

export function Card({
  title,
  subtitle,
  actions,
  children,
  className = "",
}: {
  title?: ReactNode;
  subtitle?: ReactNode;
  actions?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section className={`card ${className}`}>
      {(title || actions) && (
        <header className="flex items-start justify-between gap-4 border-b border-ink-700 px-5 py-4">
          <div>
            {title && <h2 className="text-sm font-semibold text-slate-100">{title}</h2>}
            {subtitle && <p className="mt-0.5 text-xs text-slate-400">{subtitle}</p>}
          </div>
          {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
        </header>
      )}
      <div className="px-5 py-4">{children}</div>
    </section>
  );
}

export function StatTile({
  label,
  value,
  hint,
  tone = "default",
}: {
  label: string;
  value: ReactNode;
  hint?: ReactNode;
  tone?: "default" | "good" | "warn" | "bad";
}) {
  const tones: Record<string, string> = {
    default: "text-slate-100",
    good: "text-emerald-300",
    warn: "text-amber-300",
    bad: "text-rose-300",
  };
  return (
    <div className="card px-4 py-3">
      <div className="text-xs uppercase tracking-wide text-slate-400">{label}</div>
      <div className={`mt-1 text-2xl font-semibold ${tones[tone]}`}>{value}</div>
      {hint && <div className="mt-1 text-xs text-slate-500">{hint}</div>}
    </div>
  );
}

export function Bar({ ratio, tone }: { ratio: number; tone?: "good" | "warn" | "bad" }) {
  const pct = Math.min(100, Math.max(0, ratio * 100));
  const auto = pct >= 90 ? "bad" : pct >= 75 ? "warn" : "good";
  const colors = {
    good: "bg-emerald-500",
    warn: "bg-amber-500",
    bad: "bg-rose-500",
  } as const;
  return (
    <div className="h-2 w-full overflow-hidden rounded-full bg-ink-700">
      <div className={`h-full rounded-full ${colors[tone ?? auto]}`} style={{ width: `${pct}%` }} />
    </div>
  );
}

const severityStyles: Record<Severity, string> = {
  high: "border-rose-500/40 bg-rose-500/10 text-rose-200",
  warn: "border-amber-500/40 bg-amber-500/10 text-amber-200",
  info: "border-sky-500/40 bg-sky-500/10 text-sky-200",
};

export function SeverityBadge({ severity }: { severity: Severity }) {
  const Icon = severity === "high" ? ShieldAlert : severity === "warn" ? AlertTriangle : Info;
  return (
    <span className={`chip ${severityStyles[severity]}`}>
      <Icon size={12} />
      {severity}
    </span>
  );
}

export function StateBadge({ state, health }: { state: string; health?: string }) {
  const label = health && health !== "" ? `${state} · ${health}` : state;
  const tone =
    health === "unhealthy" || state === "dead"
      ? "border-rose-500/40 bg-rose-500/10 text-rose-200"
      : state === "running"
        ? "border-emerald-500/40 bg-emerald-500/10 text-emerald-200"
        : state === "restarting" || health === "starting"
          ? "border-amber-500/40 bg-amber-500/10 text-amber-200"
          : "border-ink-600 bg-ink-800 text-slate-300";
  return <span className={`chip ${tone}`}>{label}</span>;
}

export function Chip({ children, tone = "muted" }: { children: ReactNode; tone?: "muted" | "accent" | "danger" }) {
  const tones = {
    muted: "border-ink-600 bg-ink-800/70 text-slate-300",
    accent: "border-sky-500/40 bg-sky-500/10 text-sky-200",
    danger: "border-rose-500/40 bg-rose-500/10 text-rose-200",
  } as const;
  return <span className={`chip font-mono ${tones[tone]}`}>{children}</span>;
}

export function Toggle({
  checked,
  onChange,
  label,
  hint,
  disabled,
}: {
  checked: boolean;
  onChange: (value: boolean) => void;
  label: string;
  hint?: string;
  disabled?: boolean;
}) {
  return (
    <label
      className={`flex items-start gap-3 rounded-lg border border-ink-700 bg-ink-850/60 px-3 py-2.5 ${
        disabled ? "opacity-50" : "cursor-pointer hover:border-ink-600"
      }`}
    >
      <button
        type="button"
        role="switch"
        aria-checked={checked}
        disabled={disabled}
        onClick={() => onChange(!checked)}
        className={`mt-0.5 h-5 w-9 shrink-0 rounded-full border transition-colors ${
          checked ? "border-sky-400/60 bg-sky-500/70" : "border-ink-600 bg-ink-700"
        }`}
      >
        <span
          className={`block h-4 w-4 rounded-full bg-white transition-transform ${
            checked ? "translate-x-4" : "translate-x-0.5"
          }`}
        />
      </button>
      <span>
        <span className="block text-sm text-slate-200">{label}</span>
        {hint && <span className="mt-0.5 block text-xs text-slate-500">{hint}</span>}
      </span>
    </label>
  );
}

export function Spinner({ label }: { label?: string }) {
  return (
    <div className="flex items-center gap-2 py-8 text-sm text-slate-400">
      <Loader2 className="animate-spin" size={16} />
      {label ?? "Loading"}
    </div>
  );
}

export function ErrorBanner({ message }: { message: string }) {
  return (
    <div className="card border-rose-500/40 bg-rose-500/10 px-4 py-3 text-sm text-rose-200">
      {message}
    </div>
  );
}

export function Empty({ children }: { children: ReactNode }) {
  return <p className="py-6 text-center text-sm text-slate-500">{children}</p>;
}
