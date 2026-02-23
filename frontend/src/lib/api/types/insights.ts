export interface Insight {
  id: number;
  type: InsightType;
  date: string;
  project: string | null;
  agent: string;
  model: string | null;
  prompt: string | null;
  content: string;
  created_at: string;
}

export type InsightType =
  | "daily_activity"
  | "agent_analysis";

export interface InsightsResponse {
  insights: Insight[];
}

export type AgentName = "claude" | "codex" | "gemini";

export interface GenerateInsightRequest {
  type: InsightType;
  date: string;
  project?: string;
  prompt?: string;
  agent?: AgentName;
}
