<script lang="ts">
  import type { Session } from "../../api/types.js";
  import { sessions, isRecentlyActive } from "../../stores/sessions.svelte.js";
  import { formatRelativeTime, truncate } from "../../utils/format.js";
  import { agentColor as getAgentColor } from "../../utils/agents.js";

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

  let recentlyActive = $derived(isRecentlyActive(session));

  let agentColor = $derived(
    getAgentColor(session.agent),
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
  <div class="agent-indicator" style:--agent-c={agentColor}>
    <span
      class="agent-dot"
      class:recently-active={recentlyActive}
    ></span>
    <span class="agent-label">{session.agent}</span>
  </div>
  <div class="session-info">
    <div class="session-name">{displayName}</div>
    <div class="session-meta">
      <span class="session-project">{session.project}</span>
      <span class="session-time">{timeStr}</span>
      <span class="session-count">{session.user_message_count}</span>
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
    gap: 10px;
    width: 100%;
    height: 42px;
    padding: 0 14px;
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

  .agent-indicator {
    display: flex;
    align-items: center;
    gap: 4px;
    flex-shrink: 0;
    max-width: 72px;
  }

  .agent-dot {
    width: 5px;
    height: 5px;
    border-radius: 50%;
    background: var(--agent-c);
    flex-shrink: 0;
  }

  .agent-dot.recently-active {
    animation: pulse-glow 3s ease-in-out infinite;
    will-change: box-shadow;
  }

  @keyframes pulse-glow {
    0%,
    100% {
      box-shadow: 0 0 0 0 transparent;
    }
    50% {
      box-shadow: 0 0 6px 3px color-mix(
        in srgb,
        var(--accent-green) 40%,
        transparent
      );
    }
  }

  .agent-label {
    font-size: 9px;
    font-weight: 550;
    color: var(--agent-c);
    text-transform: capitalize;
    letter-spacing: 0.01em;
    line-height: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .session-info {
    min-width: 0;
    flex: 1;
  }

  .session-name {
    font-size: 12px;
    font-weight: 450;
    color: var(--text-primary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    line-height: 1.3;
    letter-spacing: -0.005em;
  }

  .session-meta {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 10px;
    color: var(--text-muted);
    line-height: 1.3;
    letter-spacing: 0.01em;
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
