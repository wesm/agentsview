/** Analytics types — match Go structs in internal/db/analytics.go */

export type Granularity = "day" | "week" | "month";
export type HeatmapMetric = "messages" | "sessions";
export type TopSessionsMetric = "messages" | "duration";

export interface AgentSummary {
  sessions: number;
  messages: number;
}

export interface AnalyticsSummary {
  total_sessions: number;
  total_messages: number;
  active_projects: number;
  active_days: number;
  avg_messages: number;
  median_messages: number;
  p90_messages: number;
  most_active_project: string;
  concentration: number;
  agents: Record<string, AgentSummary>;
}

export interface ActivityEntry {
  date: string;
  sessions: number;
  messages: number;
  user_messages: number;
  assistant_messages: number;
  tool_calls: number;
  thinking_messages: number;
  by_agent: Record<string, number>;
}

export interface ActivityResponse {
  granularity: string;
  series: ActivityEntry[];
}

export interface HeatmapEntry {
  date: string;
  value: number;
  level: number;
}

export interface HeatmapLevels {
  l1: number;
  l2: number;
  l3: number;
  l4: number;
}

export interface HeatmapResponse {
  metric: string;
  entries: HeatmapEntry[];
  levels: HeatmapLevels;
  entries_from: string;
}

export interface ProjectAnalytics {
  name: string;
  sessions: number;
  messages: number;
  first_session: string;
  last_session: string;
  avg_messages: number;
  median_messages: number;
  agents: Record<string, number>;
  daily_trend: number;
}

export interface ProjectsAnalyticsResponse {
  projects: ProjectAnalytics[];
}

export interface HourOfWeekCell {
  day_of_week: number;
  hour: number;
  messages: number;
}

export interface HourOfWeekResponse {
  cells: HourOfWeekCell[];
}

export interface DistributionBucket {
  label: string;
  count: number;
}

export interface SessionShapeResponse {
  count: number;
  length_distribution: DistributionBucket[];
  duration_distribution: DistributionBucket[];
  autonomy_distribution: DistributionBucket[];
}

export interface Percentiles {
  p50: number;
  p90: number;
}

export interface VelocityOverview {
  turn_cycle_sec: Percentiles;
  first_response_sec: Percentiles;
  msgs_per_active_min: number;
  chars_per_active_min: number;
  tool_calls_per_active_min: number;
}

export interface VelocityBreakdown {
  label: string;
  sessions: number;
  overview: VelocityOverview;
}

export interface VelocityResponse {
  overall: VelocityOverview;
  by_agent: VelocityBreakdown[];
  by_complexity: VelocityBreakdown[];
}

export interface TopSession {
  id: string;
  project: string;
  first_message: string | null;
  message_count: number;
  duration_min: number;
}

export interface TopSessionsResponse {
  metric: string;
  sessions: TopSession[];
}

export interface ToolCategoryCount {
  category: string;
  count: number;
  pct: number;
}

export interface ToolAgentBreakdown {
  agent: string;
  total: number;
  categories: ToolCategoryCount[];
}

export interface ToolTrendEntry {
  date: string;
  by_category: Record<string, number>;
}

export interface ToolsAnalyticsResponse {
  total_calls: number;
  by_category: ToolCategoryCount[];
  by_agent: ToolAgentBreakdown[];
  trend: ToolTrendEntry[];
}
