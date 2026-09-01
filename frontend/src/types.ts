export type Severity = "high" | "warn" | "info";

export interface Finding {
  id: string;
  severity: Severity;
  category: string;
  target: string;
  title: string;
  detail: string;
  hint: string;
}

export interface CheckReport {
  findings: Finding[];
  summary: { high: number; warn: number; info: number };
}

export interface PortView {
  ip: string;
  public: number;
  private: number;
  proto: string;
  exposed: boolean;
}

export interface MountView {
  type: string;
  name: string;
  source: string;
  destination: string;
  rw: boolean;
}

export interface ContainerView {
  id: string;
  name: string;
  image: string;
  project: string;
  service: string;
  state: string;
  status: string;
  health: string;
  hasHealthcheck: boolean;
  restartPolicy: string;
  restartCount: number;
  privileged: boolean;
  dockerSocket: boolean;
  logDriver: string;
  logMaxSize: string;
  createdAt: string;
  cpuPct?: number;
  memUsage?: number;
  memLimit?: number;
  memPct?: number;
  ports: PortView[];
  networks: string[];
  mounts: MountView[];
}

export interface StackView {
  name: string;
  workingDir: string;
  configFiles: string[];
  running: number;
  total: number;
  deployed: boolean;
  cpuPct?: number;
  memUsage?: number;
  memPct?: number;
  networks: string[];
  volumes: string[];
  containers: ContainerView[];
}

export interface PruneTargets {
  buildCache: boolean;
  danglingImages: boolean;
  unusedImages: boolean;
  stoppedContainers: boolean;
  networks: boolean;
  volumes: boolean;
}

export type PruneMode = "interval" | "daily" | "weekly";

export interface PruneConfig {
  enabled: boolean;
  mode: PruneMode;
  intervalHours: number;
  timeOfDay: string;
  weekday: number;
  timezone: string;
  targets: PruneTargets;
  minAgeHours: number;
  keepPatterns: string[];
  dryRun: boolean;
}

export interface PruneItem {
  kind: string;
  ref: string;
  size: number;
  removed: boolean;
  reason?: string;
}

export interface PruneRun {
  id: string;
  trigger: string;
  dryRun: boolean;
  startedAt: string;
  finishedAt: string;
  reclaimed: number;
  items: PruneItem[];
  errors: string[];
  targets: PruneTargets;
}

export interface PruneStatus {
  enabled: boolean;
  running: boolean;
  nextRun: string | null;
  lastRun: string | null;
  lastResult: PruneRun | null;
  config: PruneConfig;
}

export interface DiskStat {
  path: string;
  total: number;
  used: number;
  free: number;
  usedRatio: number;
}

export interface StorageTotals {
  imagesSize: number;
  imagesReclaimable: number;
  volumesSize: number;
  volumesReclaimable: number;
  buildCacheSize: number;
  buildCacheReclaimable: number;
}

export interface Overview {
  takenAt: string;
  host: {
    name: string;
    dockerVersion: string;
    os: string;
    kernel: string;
    arch: string;
    cpus: number;
    memTotal: number;
    storageDriver: string;
  };
  disk: DiskStat;
  counts: {
    stacks: number;
    containersRunning: number;
    containersStopped: number;
    images: number;
    volumes: number;
    networks: number;
  };
  storage: StorageTotals;
  checks: { high: number; warn: number; info: number };
  topFindings: Finding[];
  prune: PruneStatus;
}

export interface ResourceRow {
  name: string;
  ref: string;
  size: number;
  inUse: boolean;
  createdAt: string;
  extra: string;
  protected: boolean;
}

export interface Resources {
  images: ResourceRow[];
  volumes: ResourceRow[];
  networks: ResourceRow[];
  totals: StorageTotals;
}

export interface ContainerStat {
  id: string;
  name: string;
  cpuPct: number;
  memUsage: number;
  memLimit: number;
  memPct: number;
}

export interface HostSample {
  at: string;
  cpuPct: number;
  memUsed: number;
  memTotal: number;
  memPct: number;
}

export interface MetricsSnapshot {
  takenAt: string;
  host: HostSample;
  history: HostSample[];
  containers: ContainerStat[];
  disk: DiskStat;
}

export interface ActionResult {
  ok: boolean;
  command: string;
  output: string;
}

export interface ActivityEvent {
  time: string;
  type: string;
  action: string;
  name: string;
  detail: string;
}

export interface Alert {
  id: string;
  severity: "high" | "warn" | "info";
  kind: string;
  target: string;
  title: string;
  detail: string;
}
