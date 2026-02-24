<script lang="ts">
  import { tick } from "svelte";
  import { ui } from "../../stores/ui.svelte.js";
  import { sessions } from "../../stores/sessions.svelte.js";
  import { searchStore } from "../../stores/search.svelte.js";
  import { messages } from "../../stores/messages.svelte.js";
  import {
    formatRelativeTime,
    truncate,
    sanitizeSnippet,
  } from "../../utils/format.js";
  import type { Session, SearchResult } from "../../api/types.js";

  let inputRef: HTMLInputElement | undefined = $state(undefined);
  let selectedIndex: number = $state(0);
  let inputValue: string = $state("");

  // Filtered recent sessions (client-side filter)
  let recentSessions = $derived.by(() => {
    if (inputValue.length > 0 && inputValue.length < 3) {
      const q = inputValue.toLowerCase();
      return sessions.sessions
        .filter(
          (s) =>
            s.project.toLowerCase().includes(q) ||
            (s.first_message?.toLowerCase().includes(q) ?? false),
        )
        .slice(0, 10);
    }
    if (!inputValue) {
      return sessions.sessions.slice(0, 10);
    }
    return [];
  });

  // Combined results: search results when query >= 3 chars, else recent
  let showSearchResults = $derived(inputValue.length >= 3);

  let totalItems = $derived(
    showSearchResults
      ? searchStore.results.length
      : recentSessions.length,
  );

  function handleInput(e: Event) {
    const target = e.target as HTMLInputElement;
    inputValue = target.value;
    selectedIndex = 0;

    if (inputValue.length >= 3) {
      searchStore.search(inputValue, sessions.filters.project);
    } else {
      searchStore.clear();
    }
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      selectedIndex = Math.min(selectedIndex + 1, totalItems - 1);
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      selectedIndex = Math.max(selectedIndex - 1, 0);
    } else if (e.key === "Enter") {
      e.preventDefault();
      selectCurrent();
    } else if (e.key === "Escape") {
      e.preventDefault();
      close();
    }
  }

  function selectCurrent() {
    if (showSearchResults) {
      const result = searchStore.results[selectedIndex];
      if (result) {
        selectSearchResult(result);
      }
    } else {
      const session = recentSessions[selectedIndex];
      if (session) {
        selectSession(session);
      }
    }
  }

  function selectSession(s: Session) {
    sessions.selectSession(s.id);
    close();
  }

  function selectSearchResult(r: SearchResult) {
    sessions.selectSession(r.session_id);
    ui.scrollToOrdinal(r.ordinal, r.session_id);
    close();
  }

  function close() {
    inputValue = "";
    searchStore.clear();
    ui.activeModal = null;
  }

  function handleOverlayClick(e: MouseEvent) {
    if ((e.target as HTMLElement).classList.contains("palette-overlay")) {
      close();
    }
  }

  $effect(() => {
    if (inputRef) {
      inputRef.focus();
    }
  });

  $effect(() => {
    const _idx = selectedIndex;
    tick().then(() => {
      const el = document.querySelector(
        ".palette-results .palette-item.selected",
      );
      if (el) el.scrollIntoView({ block: "nearest" });
    });
  });
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="palette-overlay"
  onclick={handleOverlayClick}
  onkeydown={handleKeydown}
>
  <div class="palette">
    <div class="palette-input-wrap">
      <svg class="search-icon" width="14" height="14" viewBox="0 0 16 16" fill="currentColor">
        <path d="M11.742 10.344a6.5 6.5 0 10-1.397 1.398h-.001l3.85 3.85a1 1 0 001.415-1.414l-3.85-3.85zm-5.44.656a5 5 0 110-10 5 5 0 010 10z"/>
      </svg>
      <input
        bind:this={inputRef}
        type="text"
        class="palette-input"
        placeholder="Search sessions and messages..."
        value={inputValue}
        oninput={handleInput}
      />
      <kbd class="esc-hint">Esc</kbd>
    </div>

    <div class="palette-results">
      {#if showSearchResults}
        {#if searchStore.isSearching}
          <div class="palette-empty">Searching...</div>
        {:else if searchStore.results.length === 0}
          <div class="palette-empty">No results</div>
        {:else}
          {#each searchStore.results as result, i}
            <button
              class="palette-item"
              class:selected={i === selectedIndex}
              onclick={() => selectSearchResult(result)}
              onmouseenter={() => (selectedIndex = i)}
            >
              <span class="item-role" class:user={result.role === "user"}>
                {result.role === "user" ? "U" : "A"}
              </span>
              <span class="item-text">
                {@html sanitizeSnippet(result.snippet)}
              </span>
              <span class="item-meta">
                {truncate(result.project, 20)}
              </span>
            </button>
          {/each}
        {/if}
      {:else}
        <div class="palette-section-label">Recent Sessions</div>
        {#each recentSessions as session, i}
          <button
            class="palette-item"
            class:selected={i === selectedIndex}
            onclick={() => selectSession(session)}
            onmouseenter={() => (selectedIndex = i)}
          >
            <span class="item-dot" style:background={
              session.agent === "codex"
                ? "var(--accent-green)"
                : session.agent === "opencode"
                  ? "var(--accent-purple)"
                  : "var(--accent-blue)"
            }></span>
            <span class="item-text">
              {session.first_message
                ? truncate(session.first_message, 60)
                : session.project}
            </span>
            <span class="item-meta">
              {formatRelativeTime(session.ended_at ?? session.started_at)}
            </span>
          </button>
        {/each}
      {/if}
    </div>
  </div>
</div>

<style>
  .palette-overlay {
    position: fixed;
    inset: 0;
    background: var(--overlay-bg);
    display: flex;
    justify-content: center;
    padding-top: 20vh;
    z-index: 100;
  }

  .palette {
    width: 560px;
    max-height: 400px;
    background: var(--bg-surface);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-md);
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }

  .palette-input-wrap {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 14px;
    border-bottom: 1px solid var(--border-default);
  }

  .search-icon {
    flex-shrink: 0;
    color: var(--text-muted);
  }

  .palette-input {
    flex: 1;
    background: none;
    border: none;
    font-size: 14px;
    color: var(--text-primary);
    outline: none;
  }

  .palette-input::placeholder {
    color: var(--text-muted);
  }

  .esc-hint {
    font-size: 10px;
    padding: 1px 5px;
    border: 1px solid var(--border-default);
    border-radius: var(--radius-sm);
    color: var(--text-muted);
    background: var(--bg-inset);
    font-family: var(--font-sans);
  }

  .palette-results {
    overflow-y: auto;
    flex: 1;
    padding: 4px 0;
  }

  .palette-section-label {
    padding: 6px 14px 4px;
    font-size: 10px;
    font-weight: 600;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .palette-item {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    padding: 6px 14px;
    text-align: left;
    font-size: 13px;
    color: var(--text-primary);
    transition: background 0.05s;
  }

  .palette-item:hover,
  .palette-item.selected {
    background: var(--bg-surface-hover);
  }

  .item-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    flex-shrink: 0;
  }

  .item-role {
    width: 18px;
    height: 18px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: var(--radius-sm);
    font-size: 10px;
    font-weight: 700;
    flex-shrink: 0;
    background: var(--assistant-bg);
    color: var(--accent-purple);
  }

  .item-role.user {
    background: var(--user-bg);
    color: var(--accent-blue);
  }

  .item-text {
    flex: 1;
    min-width: 0;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .item-meta {
    font-size: 11px;
    color: var(--text-muted);
    white-space: nowrap;
    flex-shrink: 0;
  }

  .palette-empty {
    padding: 16px;
    text-align: center;
    color: var(--text-muted);
    font-size: 13px;
  }
</style>
