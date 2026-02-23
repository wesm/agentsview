<script lang="ts">
  import { sync } from "../../stores/sync.svelte.js";
  import { formatNumber, formatRelativeTime } from "../../utils/format.js";

  let progressText = $derived.by(() => {
    if (!sync.syncing || !sync.progress) return null;
    const p = sync.progress;
    if (p.phase === "scan") {
      return `Scanning ${p.current_project || ""}...`;
    }
    if (p.phase === "parse") {
      const pct = p.sessions_total > 0
        ? Math.round((p.sessions_done / p.sessions_total) * 100)
        : 0;
      return `Syncing ${pct}% (${p.sessions_done}/${p.sessions_total})`;
    }
    return "Syncing...";
  });
</script>

<footer class="status-bar">
  <div class="status-left">
    {#if sync.stats}
      <span>{formatNumber(sync.stats.session_count)} sessions</span>
      <span class="sep">&middot;</span>
      <span>{formatNumber(sync.stats.message_count)} messages</span>
      <span class="sep">&middot;</span>
      <span>{formatNumber(sync.stats.project_count)} projects</span>
    {/if}
  </div>

  <div class="status-right">
    {#if sync.versionMismatch}
      <button
        class="version-warn"
        onclick={() => window.location.reload()}
        title="Frontend and backend versions differ. Click to reload."
      >
        version mismatch - reload
      </button>
    {/if}
    {#if progressText}
      {#if sync.versionMismatch}<span class="sep">&middot;</span>{/if}
      <span class="sync-progress">{progressText}</span>
    {:else if sync.lastSync}
      {#if sync.versionMismatch}<span class="sep">&middot;</span>{/if}
      <span>synced {formatRelativeTime(sync.lastSync)}</span>
    {/if}
    {#if sync.serverVersion}
      {#if sync.versionMismatch || progressText || sync.lastSync}
        <span class="sep">&middot;</span>
      {/if}
      <span class="version" title="Build: {sync.serverVersion.commit}">
        {sync.serverVersion.version}
      </span>
    {/if}
  </div>
</footer>

<style>
  .status-bar {
    height: 24px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0 14px;
    background: var(--bg-surface);
    border-top: 1px solid var(--border-default);
    font-size: 10px;
    color: var(--text-muted);
    flex-shrink: 0;
    letter-spacing: 0.01em;
  }

  .status-left,
  .status-right {
    display: flex;
    align-items: center;
    gap: 4px;
  }

  .sep {
    color: var(--border-default);
  }

  .sync-progress {
    color: var(--accent-green);
  }

  .version-warn {
    color: var(--accent-red);
    font-size: 10px;
    cursor: pointer;
    font-weight: 500;
  }

  .version-warn:hover {
    text-decoration: underline;
  }

  .version {
    font-family: var(--font-mono);
  }
</style>
