import { NavLink, Route, Routes } from "react-router-dom";
import {
  Activity,
  Boxes,
  HardDrive,
  LayoutDashboard,
  ShieldCheck,
  Trash2,
} from "lucide-react";
import Dashboard from "./pages/Dashboard";
import Stacks from "./pages/Stacks";
import StackDetail from "./pages/StackDetail";
import Metrics from "./pages/Metrics";
import Checks from "./pages/Checks";
import Resources from "./pages/Resources";
import Prune from "./pages/Prune";

const nav = [
  { to: "/", label: "Dashboard", icon: LayoutDashboard, end: true },
  { to: "/stacks", label: "Stacks", icon: Boxes, end: false },
  { to: "/metrics", label: "Metrics", icon: Activity, end: false },
  { to: "/checks", label: "Checks", icon: ShieldCheck, end: false },
  { to: "/resources", label: "Resources", icon: HardDrive, end: false },
  { to: "/prune", label: "Auto-prune", icon: Trash2, end: false },
];

export default function App() {
  return (
    <div className="flex min-h-full flex-col md:flex-row">
      <aside className="shrink-0 border-b border-ink-800 bg-ink-900/70 md:min-h-screen md:w-60 md:border-b-0 md:border-r">
        <div className="flex items-center gap-2 px-5 py-5">
          <div className="grid h-8 w-8 place-items-center rounded-lg bg-sky-500/20 text-sky-300">
            <Boxes size={18} />
          </div>
          <div>
            <div className="text-sm font-semibold text-slate-100">Chelops Monitoring</div>
            <div className="text-[11px] text-slate-500">stacks · metrics · prune</div>
          </div>
        </div>
        <nav className="flex gap-1 overflow-x-auto px-3 pb-3 md:flex-col md:overflow-visible">
          {nav.map(({ to, label, icon: Icon, end }) => (
            <NavLink
              key={to}
              to={to}
              end={end}
              className={({ isActive }) =>
                `flex items-center gap-2 whitespace-nowrap rounded-lg px-3 py-2 text-sm transition-colors ${
                  isActive
                    ? "bg-sky-500/15 text-sky-200"
                    : "text-slate-400 hover:bg-ink-800 hover:text-slate-200"
                }`
              }
            >
              <Icon size={16} />
              {label}
            </NavLink>
          ))}
        </nav>
      </aside>

      <main className="min-w-0 flex-1 px-4 py-6 md:px-8">
        <div className="mx-auto max-w-6xl">
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/stacks" element={<Stacks />} />
            <Route path="/stacks/:name" element={<StackDetail />} />
            <Route path="/metrics" element={<Metrics />} />
            <Route path="/checks" element={<Checks />} />
            <Route path="/resources" element={<Resources />} />
            <Route path="/prune" element={<Prune />} />
            <Route
              path="*"
              element={<p className="text-sm text-slate-400">Page not found.</p>}
            />
          </Routes>
        </div>
      </main>
    </div>
  );
}
