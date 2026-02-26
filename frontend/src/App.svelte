<script lang="ts">
  import { onMount, untrack } from "svelte";
  import AppHeader from "./lib/components/layout/AppHeader.svelte";
  import ThreeColumnLayout from "./lib/components/layout/ThreeColumnLayout.svelte";
  import StatusBar from "./lib/components/layout/StatusBar.svelte";
  import SessionList from "./lib/components/sidebar/SessionList.svelte";
  import MessageList from "./lib/components/content/MessageList.svelte";
  import CommandPalette from "./lib/components/command-palette/CommandPalette.svelte";
  import ShortcutsModal from "./lib/components/modals/ShortcutsModal.svelte";
  import PublishModal from "./lib/components/modals/PublishModal.svelte";
  import ResyncModal from "./lib/components/modals/ResyncModal.svelte";
  import AnalyticsPage from "./lib/components/analytics/AnalyticsPage.svelte";
  import InsightsPage from "./lib/components/insights/InsightsPage.svelte";
  import { sessions } from "./lib/stores/sessions.svelte.js";
  import { messages } from "./lib/stores/messages.svelte.js";
  import { sync } from "./lib/stores/sync.svelte.js";
  import { ui } from "./lib/stores/ui.svelte.js";
  import { router } from "./lib/stores/router.svelte.js";
  import { registerShortcuts } from "./lib/utils/keyboard.js";
  import { copyToClipboard } from "./lib/utils/clipboard.js";
  import type { DisplayItem } from "./lib/utils/display-items.js";

  let copiedSessionId = $state("");

  function sessionDisplayId(id: string): string {
    const idx = id.indexOf(":");
    return idx >= 0 ? id.slice(idx + 1) : id;
  }

  let messageListRef:
    | {
        scrollToOrdinal: (o: number) => void;
        getDisplayItems: () => DisplayItem[];
      }
    | undefined = $state(undefined);

  // Load active session's messages when selection changes.
  // Only track activeSessionId — untrack the rest to prevent
  // reactive loops from messages.loading / messages.messages.
  $effect(() => {
    const id = sessions.activeSessionId;
    untrack(() => {
      // Preserve selection when a pending scroll is queued
      // for this specific session (e.g. search result
      // navigation sets session + ordinal before this effect
      // fires). Clear if the pending scroll targets a
      // different session or there is no pending scroll.
      const pendingMatchesSession =
        ui.pendingScrollOrdinal !== null &&
        (ui.pendingScrollSession === null ||
          ui.pendingScrollSession === id);
      if (!pendingMatchesSession) {
        ui.clearSelection();
        ui.pendingScrollOrdinal = null;
        ui.pendingScrollSession = null;
      }
      if (id) {
        messages.loadSession(id);
        sync.watchSession(id, () => messages.reload());
      } else {
        messages.clear();
        sync.unwatchSession();
      }
    });
  });

  // Scroll to pending ordinal once messages finish loading.
  // If the target message is hidden (thinking-only with thinking
  // disabled), auto-enable thinking so the message becomes visible.
  $effect(() => {
    const ordinal = ui.pendingScrollOrdinal;
    const loading = messages.loading;
    const showThinking = ui.showThinking;
    untrack(() => {
      if (ordinal === null || loading || !messageListRef) return;

      const items = messageListRef.getDisplayItems();
      const found = items.some((item) =>
        item.ordinals.includes(ordinal),
      );

      if (!found && !showThinking) {
        // Only auto-enable thinking if the ordinal is loaded
        // but filtered out. If it's outside the loaded window,
        // try loading it first instead of changing the filter.
        const loaded = messages.messages.some(
          (m) => m.ordinal === ordinal,
        );
        if (loaded) {
          ui.showThinking = true;
          return; // effect re-runs with showThinking=true
        }
      }

      messageListRef.scrollToOrdinal(ordinal);
      // Ensure highlight is set (the session-change effect
      // may have cleared it before this effect ran).
      ui.selectedOrdinal = ordinal;
      ui.pendingScrollOrdinal = null;
      ui.pendingScrollSession = null;
    });
  });

  function navigateMessage(delta: number) {
    const items = messageListRef?.getDisplayItems();
    if (!items || items.length === 0) return;

    const sorted = ui.sortNewestFirst
      ? [...items].reverse()
      : items;

    const selected = ui.selectedOrdinal;
    if (selected === null) {
      const first = sorted[0]!;
      ui.selectOrdinal(first.ordinals[0]!);
      messageListRef?.scrollToOrdinal(first.ordinals[0]!);
      return;
    }

    const curIdx = sorted.findIndex((item) =>
      item.ordinals.includes(selected),
    );
    const nextIdx = Math.max(
      0,
      Math.min(sorted.length - 1, curIdx + delta),
    );
    if (nextIdx === curIdx) return;

    const next = sorted[nextIdx]!;
    ui.selectOrdinal(next.ordinals[0]!);
    messageListRef?.scrollToOrdinal(next.ordinals[0]!);
  }

  // React to route changes: initialize session filters from URL params
  $effect(() => {
    const _route = router.route;
    const params = router.params;
    untrack(() => {
      sessions.initFromParams(params);
      sessions.load();
      sessions.loadProjects();
    });
  });

  onMount(() => {
    sync.loadStatus();
    sync.loadStats();
    sync.loadVersion();
    sync.startPolling();

    const cleanup = registerShortcuts({ navigateMessage });
    return () => {
      cleanup();
      sync.stopPolling();
      sync.unwatchSession();
    };
  });

</script>

<AppHeader />

{#if router.route === "insights"}
  <InsightsPage />
{:else}
  <ThreeColumnLayout>
    {#snippet sidebar()}
      <SessionList />
    {/snippet}

    {#snippet content()}
      {#if sessions.activeSessionId}
        {@const session = sessions.activeSession}
        <div class="session-breadcrumb">
          <button
            class="breadcrumb-link"
            onclick={() => sessions.deselectSession()}
          >Sessions</button>
          <span class="breadcrumb-sep">/</span>
          <span class="breadcrumb-current">
            {session?.project ?? ""}
          </span>
          {#if session}
            <span class="breadcrumb-meta">
              <span
                class="agent-badge"
                class:agent-claude={session.agent === "claude"}
                class:agent-codex={session.agent === "codex"}
                class:agent-copilot={session.agent === "copilot"}
                class:agent-gemini={session.agent === "gemini"}
                class:agent-opencode={session.agent === "opencode"}
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
              {#if session.agent === "claude" || session.agent === "codex"}
                {@const rawId = sessionDisplayId(session.id)}
                <button
                  class="session-id"
                  title={rawId}
                  onclick={async () => {
                    const ok = await copyToClipboard(rawId);
                    if (ok) {
                      const id = session.id;
                      copiedSessionId = id;
                      setTimeout(() => {
                        if (copiedSessionId === id) copiedSessionId = "";
                      }, 1500);
                    }
                  }}
                >
                  {copiedSessionId === session.id ? "Copied!" : rawId.slice(0, 8)}
                </button>
              {/if}
            </span>
          {/if}
        </div>
        <MessageList bind:this={messageListRef} />
      {:else}
        <AnalyticsPage />
      {/if}
    {/snippet}
  </ThreeColumnLayout>
{/if}

<StatusBar />

{#if ui.activeModal === "commandPalette"}
  <CommandPalette />
{/if}

{#if ui.activeModal === "shortcuts"}
  <ShortcutsModal />
{/if}

{#if ui.activeModal === "publish"}
  <PublishModal />
{/if}

{#if ui.activeModal === "resync"}
  <ResyncModal />
{/if}

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

  .agent-claude {
    background: var(--accent-blue);
  }

  .agent-codex {
    background: var(--accent-green);
  }

  .agent-copilot {
    background: var(--accent-amber);
  }

  .agent-gemini {
    background: var(--accent-rose);
  }

  .agent-opencode {
    background: var(--accent-purple);
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
