<script lang="ts">
  import type { Session } from "../../api/types.js";
  import { copyToClipboard } from "../../utils/clipboard.js";
  import { agentColor } from "../../utils/agents.js";
  import {
    supportsResume,
    buildResumeCommand,
  } from "../../utils/resume.js";

  interface Props {
    session: Session | undefined;
    onBack: () => void;
  }

  let { session, onBack }: Props = $props();
  let copiedSessionId = $state("");
  let copiedResume = $state(false);

  function sessionDisplayId(id: string): string {
    const idx = id.indexOf(":");
    return idx >= 0 ? id.slice(idx + 1) : id;
  }

  async function copySessionId(rawId: string, sessionId: string) {
    const ok = await copyToClipboard(rawId);
    if (!ok) return;

    copiedSessionId = sessionId;
    setTimeout(() => {
      if (copiedSessionId === sessionId) copiedSessionId = "";
    }, 1500);
  }

  async function handleResume() {
    if (!session) return;
    const cmd = buildResumeCommand(session.agent, session.id);
    if (!cmd) return;
    const ok = await copyToClipboard(cmd);
    if (!ok) return;

    copiedResume = true;
    setTimeout(() => {
      copiedResume = false;
    }, 1500);
  }

  const canResume = $derived(
    session ? supportsResume(session.agent) : false,
  );
</script>

<div class="session-breadcrumb">
  <button class="breadcrumb-link" onclick={onBack}>Sessions</button>
  <span class="breadcrumb-sep">/</span>
  <span class="breadcrumb-current">{session?.project ?? ""}</span>
  {#if session}
    <span class="breadcrumb-meta">
      <span
        class="agent-badge"
        style:background={agentColor(session.agent)}
      >{session.agent}</span>
      {#if session.started_at}
        <span class="session-time">
          {new Date(session.started_at).toLocaleDateString(undefined, {
            month: "short",
            day: "numeric",
          })}
          {new Date(session.started_at).toLocaleTimeString(undefined, {
            hour: "2-digit",
            minute: "2-digit",
          })}
        </span>
      {/if}
      {#if canResume}
        <button
          class="resume-btn"
          class:copied={copiedResume}
          onclick={handleResume}
          title="Copy CLI command to resume this session in your terminal"
          aria-label="Resume in terminal"
        >
          {#if copiedResume}
            <svg width="11" height="11" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
              <path d="M13.78 4.22a.75.75 0 010 1.06l-7.25 7.25a.75.75 0 01-1.06 0L2.22 9.28a.75.75 0 011.06-1.06L6 10.94l6.72-6.72a.75.75 0 011.06 0z"/>
            </svg>
            Copied!
          {:else}
            <svg width="11" height="11" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
              <path d="M0 2.75C0 1.784.784 1 1.75 1h8.5c.966 0 1.75.784 1.75 1.75v7.5A1.75 1.75 0 0110.25 12h-8.5A1.75 1.75 0 010 10.25v-7.5zm1.75-.25a.25.25 0 00-.25.25v7.5c0 .138.112.25.25.25h8.5a.25.25 0 00.25-.25v-7.5a.25.25 0 00-.25-.25h-8.5z"/>
              <path d="M3.5 4.5h5v1h-5zm0 2h5v1h-5zm0 2h3v1h-3z"/>
            </svg>
            Resume
          {/if}
        </button>
      {/if}
      {#if session.id}
        {@const rawId = sessionDisplayId(session.id)}
        <button
          class="session-id"
          title={rawId}
          onclick={() => copySessionId(rawId, session.id)}
        >
          {copiedSessionId === session.id ? "Copied!" : rawId.slice(0, 8)}
        </button>
      {/if}
    </span>
  {/if}
</div>

<style>
  .session-breadcrumb {
    display: flex;
    align-items: center;
    gap: 6px;
    height: 32px;
    padding: 0 14px;
    border-bottom: 1px solid var(--border-muted);
    flex-shrink: 0;
    font-size: 11px;
    color: var(--text-muted);
  }

  .breadcrumb-link {
    color: var(--text-muted);
    font-size: 11px;
    font-weight: 500;
    cursor: pointer;
    transition: color 0.12s;
  }

  .breadcrumb-link:hover {
    color: var(--accent-blue);
  }

  .breadcrumb-sep {
    opacity: 0.3;
    font-size: 10px;
  }

  .breadcrumb-current {
    color: var(--text-primary);
    font-weight: 500;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    flex: 1;
    min-width: 0;
  }

  .breadcrumb-meta {
    display: flex;
    align-items: center;
    gap: 6px;
    margin-left: auto;
    flex-shrink: 0;
  }

  .agent-badge {
    font-size: 9px;
    font-weight: 600;
    padding: 1px 6px;
    border-radius: 8px;
    text-transform: uppercase;
    letter-spacing: 0.03em;
    color: white;
    flex-shrink: 0;
    background: var(--text-muted);
  }

  .session-time {
    font-size: 10px;
    color: var(--text-muted);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
    flex-shrink: 0;
  }

  .resume-btn {
    display: flex;
    align-items: center;
    gap: 4px;
    font-size: 10px;
    font-weight: 500;
    color: var(--text-muted);
    padding: 1px 7px;
    border-radius: 4px;
    background: var(--bg-tertiary);
    cursor: pointer;
    white-space: nowrap;
    flex-shrink: 0;
    transition: color 0.15s, background 0.15s;
  }

  .resume-btn:hover {
    color: var(--text-secondary);
    background: var(--bg-hover);
  }

  .resume-btn.copied {
    color: var(--accent-green);
  }

  .session-id {
    font-size: 10px;
    font-family: "SF Mono", "Menlo", "Consolas", monospace;
    color: var(--text-muted);
    cursor: pointer;
    padding: 1px 5px;
    border-radius: 4px;
    background: var(--bg-tertiary);
    transition: color 0.15s, background 0.15s;
    white-space: nowrap;
    flex-shrink: 0;
  }

  .session-id:hover {
    color: var(--text-secondary);
    background: var(--bg-hover);
  }
</style>
