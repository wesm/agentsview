export interface AgentMeta {
  name: string;
  color: string;
}

export const KNOWN_AGENTS: readonly AgentMeta[] = [
  { name: "claude", color: "var(--accent-blue)" },
  { name: "codex", color: "var(--accent-green)" },
  { name: "copilot", color: "var(--accent-amber)" },
  { name: "gemini", color: "var(--accent-rose)" },
  { name: "opencode", color: "var(--accent-purple)" },
];

const agentColorMap = new Map(
  KNOWN_AGENTS.map((a) => [a.name, a.color]),
);

export function agentColor(agent: string): string {
  return agentColorMap.get(agent) ?? "var(--accent-blue)";
}
