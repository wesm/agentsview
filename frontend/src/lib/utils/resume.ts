/** Agent types that support CLI session resumption. */
const RESUME_AGENTS: Record<
  string,
  (sessionId: string) => string
> = {
  claude: (id) => `claude --resume ${quote(id)}`,
  codex: (id) => `codex resume ${id}`,
  gemini: (id) => `gemini --resume ${id}`,
  opencode: (id) => `opencode --session ${id}`,
  amp: (id) => `amp --resume ${id}`,
};

/** Flags available for Claude Code resume. */
export interface ClaudeResumeFlags {
  skipPermissions?: boolean;
  forkSession?: boolean;
  print?: boolean;
}

function quote(s: string): string {
  // Shell-quote if the ID contains characters that need escaping
  if (/^[\w-]+$/.test(s)) return s;
  return JSON.stringify(s);
}

/**
 * Strip the agent-type prefix from a compound session ID.
 * e.g. "codex:abc123" → "abc123", "plain-uuid" → "plain-uuid"
 */
function stripIdPrefix(id: string): string {
  const idx = id.indexOf(":");
  return idx >= 0 ? id.slice(idx + 1) : id;
}

/**
 * Returns true if the given agent supports CLI session resumption.
 */
export function supportsResume(agent: string): boolean {
  return agent in RESUME_AGENTS;
}

/**
 * Build a CLI command to resume the given session in a terminal.
 *
 * @param agent - The agent type (e.g. "claude", "codex", "gemini")
 * @param sessionId - The session ID (may include agent prefix)
 * @param flags - Optional Claude-specific resume flags
 * @returns The shell command string, or null if the agent is not supported
 */
export function buildResumeCommand(
  agent: string,
  sessionId: string,
  flags?: ClaudeResumeFlags,
): string | null {
  const builder = RESUME_AGENTS[agent];
  if (!builder) return null;

  const rawId = stripIdPrefix(sessionId);
  let cmd = builder(rawId);

  if (agent === "claude" && flags) {
    if (flags.skipPermissions)
      cmd += " --dangerously-skip-permissions";
    if (flags.forkSession) cmd += " --fork-session";
    if (flags.print) cmd += " --print";
  }

  return cmd;
}
