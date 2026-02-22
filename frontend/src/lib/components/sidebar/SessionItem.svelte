<script lang="ts">
  import type { Session } from "../../api/types.js";
  import { sessions } from "../../stores/sessions.svelte.js";
  import { formatRelativeTime, truncate } from "../../utils/format.js";

  interface Props {
    session: Session;
    continuationCount?: number;
    groupSessionIds?: string[];
  }

  let {
    session,
    continuationCount = 1,
    groupSessionIds,
  }: Props = $props();

  let isActive = $derived(
    groupSessionIds
      ? groupSessionIds.includes(
          sessions.activeSessionId ?? "",
        )
      : sessions.activeSessionId === session.id,
  );

  let agentColor = $derived(
    session.agent === "codex"
      ? "var(--accent-green)"
      : "var(--accent-blue)",
  );

  let displayName = $derived(
    session.first_message
      ? truncate(session.first_message, 50)
      : truncate(session.project, 30),
  );

  let timeStr = $derived(
    formatRelativeTime(session.ended_at ?? session.started_at),
  );
</script>

<button
  class="session-item"
  class:active={isActive}
  data-session-id={session.id}
  onclick={() => sessions.selectSession(session.id)}
>
  <div class="agent-dot" style:background={agentColor}></div>
  <div class="session-info">
    <div class="session-name">{displayName}</div>
    <div class="session-meta">
      <span class="session-project">{session.project}</span>
      <span class="session-time">{timeStr}</span>
      <span class="session-count">{session.message_count}</span>
      {#if continuationCount > 1}
        <span class="continuation-badge">x{continuationCount}</span>
      {/if}
    </div>
  </div>
</button>

<style>
  .session-item {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    height: 40px;
    padding: 0 12px;
    text-align: left;
    border-left: 2px solid transparent;
    transition: background 0.1s;
  }

  .session-item:hover {
    background: var(--bg-surface-hover);
  }

  .session-item.active {
    background: var(--bg-surface-hover);
    border-left-color: var(--accent-blue);
  }

  .agent-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    flex-shrink: 0;
  }

  .session-info {
    min-width: 0;
    flex: 1;
  }

  .session-name {
    font-size: 12px;
    color: var(--text-primary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    line-height: 1.3;
  }

  .session-meta {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 10px;
    color: var(--text-muted);
    line-height: 1.3;
  }

  .session-project {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    max-width: 100px;
  }

  .session-time {
    white-space: nowrap;
    flex-shrink: 0;
  }

  .session-count {
    white-space: nowrap;
    flex-shrink: 0;
  }

  .session-count::before {
    content: "\2022 ";
  }

  .continuation-badge {
    font-size: 9px;
    font-weight: 600;
    color: var(--accent-blue);
    white-space: nowrap;
    flex-shrink: 0;
  }
</style>
