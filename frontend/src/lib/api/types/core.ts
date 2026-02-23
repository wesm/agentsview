/** Matches Go VersionInfo struct in internal/server/server.go */
export interface VersionInfo {
  version: string;
  commit: string;
  build_date: string;
}

/** Matches Go Session struct in internal/db/sessions.go */
export interface Session {
  id: string;
  project: string;
  machine: string;
  agent: string;
  first_message: string | null;
  started_at: string | null;
  ended_at: string | null;
  message_count: number;
  parent_session_id?: string;
  file_path?: string;
  file_size?: number;
  file_mtime?: number;
  created_at: string;
}

/** Matches Go SessionPage struct */
export interface SessionPage {
  sessions: Session[];
  next_cursor?: string;
  total: number;
}

/** Matches Go ProjectInfo struct */
export interface ProjectInfo {
  name: string;
  session_count: number;
}

/** Matches Go ToolCall struct in internal/db/messages.go */
export interface ToolCall {
  tool_name: string;
  category: string;
  tool_use_id?: string;
  input_json?: string;
  skill_name?: string;
  result_content_length?: number;
}

/** Matches Go Message struct in internal/db/messages.go */
export interface Message {
  id: number;
  session_id: string;
  ordinal: number;
  role: string;
  content: string;
  timestamp: string;
  has_thinking: boolean;
  has_tool_use: boolean;
  content_length: number;
  tool_calls?: ToolCall[];
}

/** Matches Go MinimapEntry struct */
export type MinimapEntry = Pick<
  Message,
  | "ordinal"
  | "role"
  | "content_length"
  | "has_thinking"
  | "has_tool_use"
>;

/** Matches Go SearchResult struct in internal/db/search.go */
export interface SearchResult {
  session_id: string;
  project: string;
  ordinal: number;
  role: string;
  timestamp: string;
  snippet: string;
  rank: number;
}

/** Matches Go Stats struct in internal/db/stats.go */
export interface Stats {
  session_count: number;
  message_count: number;
  project_count: number;
  machine_count: number;
}

export interface MessagesResponse {
  messages: Message[];
  count: number;
}

export interface MinimapResponse {
  entries: MinimapEntry[];
  count: number;
}

export interface SearchResponse {
  query: string;
  results: SearchResult[];
  count: number;
  next: number;
}

export interface ProjectsResponse {
  projects: ProjectInfo[];
}

export interface MachinesResponse {
  machines: string[];
}
