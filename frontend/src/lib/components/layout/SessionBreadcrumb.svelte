<script lang="ts">
  import type { Session } from "../../api/types.js";
  import { copyToClipboard } from "../../utils/clipboard.js";
  import { agentColor } from "../../utils/agents.js";

  interface Props {
    session: Session | undefined;
    onBack: () => void;
  }

  let { session, onBack }: Props = $props();
  let copiedSessionId = $state("");

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
