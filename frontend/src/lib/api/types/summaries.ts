export interface Summary {
  id: number;
  type: SummaryType;
  date: string;
  project: string | null;
  agent: string;
  model: string | null;
  prompt: string | null;
  content: string;
  created_at: string;
}

export type SummaryType =
  | "daily_activity"
  | "agent_analysis";

export interface SummariesResponse {
  summaries: Summary[];
}

export interface GenerateSummaryRequest {
  type: SummaryType;
  date: string;
  project?: string;
  prompt?: string;
}
