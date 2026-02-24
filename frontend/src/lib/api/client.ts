import type {
  SessionPage,
  Session,
  MessagesResponse,
  MinimapResponse,
  SearchResponse,
  ProjectsResponse,
  MachinesResponse,
  AgentsResponse,
  Stats,
  VersionInfo,
  SyncStatus,
  SyncProgress,
  SyncStats,
  PublishResponse,
  GithubConfig,
  SetGithubConfigResponse,
  AnalyticsSummary,
  ActivityResponse,
  HeatmapResponse,
  ProjectsAnalyticsResponse,
  HourOfWeekResponse,
  SessionShapeResponse,
  VelocityResponse,
  ToolsAnalyticsResponse,
  TopSessionsResponse,
  Granularity,
  HeatmapMetric,
  TopSessionsMetric,
} from "./types.js";

const BASE = "/api/v1";

async function fetchJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, init);
  if (!res.ok) {
    const body = await res.text();
    throw new Error(`API ${res.status}: ${body}`);
  }
  return res.json() as Promise<T>;
}

type QueryValue = string | number | boolean | undefined | null;

function buildQuery(params: Record<string, QueryValue>): string {
  const q = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== null && value !== "") {
      q.set(key, String(value));
    }
  }
  const qs = q.toString();
  return qs ? `?${qs}` : "";
}

/* Sessions */

export interface ListSessionsParams {
  project?: string;
  machine?: string;
  agent?: string;
  date?: string;
  date_from?: string;
  date_to?: string;
  min_messages?: number;
  max_messages?: number;
  cursor?: string;
  limit?: number;
}

export function listSessions(
  params: ListSessionsParams = {},
): Promise<SessionPage> {
  return fetchJSON(`/sessions${buildQuery({ ...params })}`);
}

export function getSession(id: string, init?: RequestInit): Promise<Session> {
  return fetchJSON(`/sessions/${id}`, init);
}

/* Messages */

export interface GetMessagesParams {
  from?: number;
  limit?: number;
  direction?: "asc" | "desc";
}

export function getMessages(
  sessionId: string,
  params: GetMessagesParams = {},
  init?: RequestInit,
): Promise<MessagesResponse> {
  return fetchJSON(
    `/sessions/${sessionId}/messages${buildQuery({ ...params })}`,
    init,
  );
}

export interface GetMinimapParams {
  from?: number;
  max?: number;
}

export function getMinimap(
  sessionId: string,
  params: GetMinimapParams = {},
): Promise<MinimapResponse> {
  return fetchJSON(
    `/sessions/${sessionId}/minimap${buildQuery({ ...params })}`,
  );
}

/* Search */

export function search(
  query: string,
  params: {
    project?: string;
    limit?: number;
    cursor?: number;
  } = {},
  init?: RequestInit,
): Promise<SearchResponse> {
  if (!query) {
    throw new Error("search query must not be empty");
  }
  return fetchJSON(`/search${buildQuery({ q: query, ...params })}`, init);
}

/* Metadata */

export function getProjects(): Promise<ProjectsResponse> {
  return fetchJSON("/projects");
}

export function getMachines(): Promise<MachinesResponse> {
  return fetchJSON("/machines");
}

export function getAgents(): Promise<AgentsResponse> {
  return fetchJSON("/agents");
}

export function getStats(): Promise<Stats> {
  return fetchJSON("/stats");
}

export function getVersion(): Promise<VersionInfo> {
  return fetchJSON("/version");
}

/* Sync */

export function getSyncStatus(): Promise<SyncStatus> {
  return fetchJSON("/sync/status");
}

export interface SyncHandle {
  abort: () => void;
  done: Promise<SyncStats>;
}

export function triggerSync(
  onProgress?: (p: SyncProgress) => void,
): SyncHandle {
  const controller = new AbortController();

  const done = (async () => {
    const res = await fetch(`${BASE}/sync`, {
      method: "POST",
      signal: controller.signal,
    });

    if (!res.ok || !res.body) {
      throw new Error(`Sync request failed: ${res.status}`);
    }

    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buf = "";
    let stats: SyncStats | undefined;

    for (;;) {
      const { done: eof, value } = await reader.read();
      if (eof) break;
      buf += decoder.decode(value, { stream: true });
      buf = buf.replaceAll("\r\n", "\n");

      const result = processFrames(buf, onProgress);
      if (result) {
        stats = result;
        reader.cancel();
        break;
      }
      const last = buf.lastIndexOf("\n\n");
      if (last !== -1) buf = buf.slice(last + 2);
    }

    if (!stats && buf.trim()) {
      stats = processFrame(buf, onProgress);
    }

    if (!stats) {
      throw new Error("Sync stream ended without done event");
    }

    return stats;
  })();

  return { abort: () => controller.abort(), done };
}

/**
 * Parse all complete SSE frames in buf.
 * Returns the SyncStats if a "done" event was received, undefined otherwise.
 */
function processFrames(
  buf: string,
  onProgress?: (p: SyncProgress) => void,
): SyncStats | undefined {
  let idx: number;
  let start = 0;
  while ((idx = buf.indexOf("\n\n", start)) !== -1) {
    const frame = buf.slice(start, idx);
    start = idx + 2;
    const stats = processFrame(frame, onProgress);
    if (stats) return stats;
  }
  return undefined;
}

/**
 * Dispatch a single SSE frame.
 * Returns the SyncStats if it was a "done" event, undefined otherwise.
 */
function processFrame(
  frame: string,
  onProgress?: (p: SyncProgress) => void,
): SyncStats | undefined {
  let event = "";
  const dataLines: string[] = [];
  for (const line of frame.split("\n")) {
    if (line.startsWith("event: ")) {
      event = line.slice(7);
    } else if (line.startsWith("data: ")) {
      dataLines.push(line.slice(6));
    } else if (line.startsWith("data:")) {
      dataLines.push(line.slice(5));
    }
  }
  const data = dataLines.join("\n");
  if (!data) return undefined;

  if (event === "progress") {
    onProgress?.(JSON.parse(data) as SyncProgress);
  } else if (event === "done") {
    return JSON.parse(data) as SyncStats;
  }
  return undefined;
}

/** Watch a session for live updates via SSE */
export function watchSession(
  sessionId: string,
  onUpdate: () => void,
): EventSource {
  const es = new EventSource(`${BASE}/sessions/${sessionId}/watch`);

  es.addEventListener("session_updated", () => {
    onUpdate();
  });

  es.onerror = () => {
    // Connection will auto-retry via EventSource spec
  };

  return es;
}

/** Get the export URL for a session */
export function getExportUrl(sessionId: string): string {
  return `${BASE}/sessions/${sessionId}/export`;
}

/* Publish / GitHub config */

export function publishSession(sessionId: string): Promise<PublishResponse> {
  return fetchJSON(`/sessions/${sessionId}/publish`, {
    method: "POST",
  });
}

export function getGithubConfig(): Promise<GithubConfig> {
  return fetchJSON("/config/github");
}

export function setGithubConfig(
  token: string,
): Promise<SetGithubConfigResponse> {
  return fetchJSON("/config/github", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ token }),
  });
}

/* Analytics */

export interface AnalyticsParams {
  from?: string;
  to?: string;
  timezone?: string;
  machine?: string;
  project?: string;
  agent?: string;
  dow?: number;
  hour?: number;
}

export function getAnalyticsSummary(
  params: AnalyticsParams,
): Promise<AnalyticsSummary> {
  return fetchJSON(`/analytics/summary${buildQuery({ ...params })}`);
}

export function getAnalyticsActivity(
  params: AnalyticsParams & {
    granularity?: Granularity;
  },
): Promise<ActivityResponse> {
  return fetchJSON(`/analytics/activity${buildQuery({ ...params })}`);
}

export function getAnalyticsHeatmap(
  params: AnalyticsParams & {
    metric?: HeatmapMetric;
  },
): Promise<HeatmapResponse> {
  return fetchJSON(`/analytics/heatmap${buildQuery({ ...params })}`);
}

export function getAnalyticsProjects(
  params: AnalyticsParams,
): Promise<ProjectsAnalyticsResponse> {
  return fetchJSON(`/analytics/projects${buildQuery({ ...params })}`);
}

export function getAnalyticsHourOfWeek(
  params: AnalyticsParams,
): Promise<HourOfWeekResponse> {
  return fetchJSON(`/analytics/hour-of-week${buildQuery({ ...params })}`);
}

export function getAnalyticsSessionShape(
  params: AnalyticsParams,
): Promise<SessionShapeResponse> {
  return fetchJSON(`/analytics/sessions${buildQuery({ ...params })}`);
}

export function getAnalyticsVelocity(
  params: AnalyticsParams,
): Promise<VelocityResponse> {
  return fetchJSON(`/analytics/velocity${buildQuery({ ...params })}`);
}

export function getAnalyticsTools(
  params: AnalyticsParams,
): Promise<ToolsAnalyticsResponse> {
  return fetchJSON(`/analytics/tools${buildQuery({ ...params })}`);
}

export function getAnalyticsTopSessions(
  params: AnalyticsParams & {
    metric?: TopSessionsMetric;
  },
): Promise<TopSessionsResponse> {
  return fetchJSON(`/analytics/top-sessions${buildQuery({ ...params })}`);
}
