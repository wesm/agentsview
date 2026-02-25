<script lang="ts">
  import { onDestroy } from "svelte";
  import { sessions } from "../../stores/sessions.svelte.js";
  import SessionItem from "./SessionItem.svelte";
  import { formatNumber } from "../../utils/format.js";
  import { KNOWN_AGENTS } from "../../utils/agents.js";

  const ITEM_HEIGHT = 40;
  const OVERSCAN = 10;

  let containerRef: HTMLDivElement | undefined = $state(undefined);
  let scrollTop = $state(0);
  let viewportHeight = $state(0);
  let scrollRaf: number | null = $state(null);
  let showFilterDropdown = $state(false);
  let filterBtnRef: HTMLButtonElement | undefined =
    $state(undefined);
  let dropdownRef: HTMLDivElement | undefined =
    $state(undefined);

  let hasFilters = $derived(sessions.hasActiveFilters);
  let isRecentlyActiveOn = $derived(
    sessions.filters.recentlyActive,
  );
  let isHideUnknownOn = $derived(
    sessions.filters.hideUnknownProject,
  );

  let groups = $derived(sessions.groupedSessions);
  let totalCount = $derived(groups.length);

  let startIndex = $derived(
    Math.max(
      0,
      Math.floor(scrollTop / ITEM_HEIGHT) - OVERSCAN,
    ),
  );

  let endIndex = $derived.by(() => {
    if (totalCount === 0) return -1;
    const visibleCount = Math.ceil(
      viewportHeight / ITEM_HEIGHT,
    );
    const last = startIndex + visibleCount + OVERSCAN * 2;
    return Math.max(
      startIndex,
      Math.min(totalCount - 1, last),
    );
  });

  let virtualRows = $derived.by(() => {
    if (totalCount === 0 || endIndex < startIndex) return [];
    const rows = [];
    for (let i = startIndex; i <= endIndex; i++) {
      rows.push({
        index: i,
        key: i,
        size: ITEM_HEIGHT,
        start: i * ITEM_HEIGHT,
      });
    }
    return rows;
  });

  let totalSize = $derived(totalCount * ITEM_HEIGHT);

  $effect(() => {
    if (!containerRef) return;
    viewportHeight = containerRef.clientHeight;
    const ro = new ResizeObserver(() => {
      if (!containerRef) return;
      viewportHeight = containerRef.clientHeight;
    });
    ro.observe(containerRef);
    return () => ro.disconnect();
  });

  // Clamp stale scrollTop when count shrinks (e.g. project filter).
  $effect(() => {
    if (!containerRef) return;
    const maxTop = Math.max(
      0,
      totalSize - containerRef.clientHeight,
    );
    if (scrollTop > maxTop) {
      scrollTop = maxTop;
      containerRef.scrollTop = maxTop;
    }
  });

  // Close filter dropdown on outside click.
  $effect(() => {
    if (!showFilterDropdown) return;
    function onClickOutside(e: MouseEvent) {
      const target = e.target as Node;
      if (
        filterBtnRef?.contains(target) ||
        dropdownRef?.contains(target)
      )
        return;
      showFilterDropdown = false;
    }
    document.addEventListener("click", onClickOutside, true);
    return () =>
      document.removeEventListener(
        "click",
        onClickOutside,
        true,
      );
  });

  // Throttle scroll position updates to one per frame.
  function handleScroll() {
    if (!containerRef) return;
    if (scrollRaf !== null) return;
    scrollRaf = requestAnimationFrame(() => {
      scrollRaf = null;
      if (!containerRef) return;
      scrollTop = containerRef.scrollTop;
    });
  }

  onDestroy(() => {
    if (scrollRaf !== null) {
      cancelAnimationFrame(scrollRaf);
      scrollRaf = null;
    }
  });
</script>

<div class="session-list-header">
  <span class="session-count">
    {formatNumber(sessions.total)} sessions
  </span>
  <div class="header-actions">
    {#if sessions.loading}
      <span class="loading-indicator">loading</span>
    {/if}
    <button
      class="filter-btn"
      bind:this={filterBtnRef}
      onclick={() =>
        (showFilterDropdown = !showFilterDropdown)}
    >
      <svg
        width="14"
        height="14"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <polygon
          points="22 3 2 3 10 12.46 10 19 14 21 14 12.46 22 3"
        />
      </svg>
      {#if hasFilters}
        <span class="filter-indicator"></span>
      {/if}
    </button>
    {#if showFilterDropdown}
      <div class="filter-dropdown" bind:this={dropdownRef}>
        <div class="filter-section">
          <div class="filter-section-label">Activity</div>
          <button
            class="filter-toggle"
            class:active={isRecentlyActiveOn}
            onclick={() =>
              sessions.setRecentlyActiveFilter(
                !isRecentlyActiveOn,
              )}
          >
            <span
              class="toggle-check"
              class:on={isRecentlyActiveOn}
            ></span>
            Recently Active
          </button>
        </div>
        <div class="filter-section">
          <div class="filter-section-label">Project</div>
          <button
            class="filter-toggle"
            class:active={isHideUnknownOn}
            onclick={() =>
              sessions.setHideUnknownProjectFilter(
                !isHideUnknownOn,
              )}
          >
            <span
              class="toggle-check"
              class:on={isHideUnknownOn}
            ></span>
            Hide unknown
          </button>
        </div>
        <div class="filter-section">
          <div class="filter-section-label">Agent</div>
          <div class="agent-buttons">
            {#each KNOWN_AGENTS as agent}
              {@const isSelected =
                sessions.filters.agent === agent.name}
              <button
                class="agent-filter-btn"
                class:active={isSelected}
                style:--agent-color={agent.color}
                onclick={() =>
                  sessions.setAgentFilter(agent.name)}
              >
                <span
                  class="agent-dot-mini"
                  style:background={agent.color}
                ></span>
                {agent.name}
              </button>
            {/each}
          </div>
        </div>
        <div class="filter-section">
          <div class="filter-section-label">Min Prompts</div>
          <div class="agent-buttons">
            {#each [2, 3, 5, 10] as n}
              <button
                class="agent-filter-btn"
                class:active={sessions.filters.minUserMessages === n}
                onclick={() =>
                  sessions.setMinUserMessagesFilter(n)}
              >
                {n}
              </button>
            {/each}
          </div>
        </div>
        {#if hasFilters}
          <button
            class="clear-filters-btn"
            onclick={() => sessions.clearSessionFilters()}
          >
            Clear filters
          </button>
        {/if}
      </div>
    {/if}
  </div>
</div>

<div
  class="session-list-scroll"
  bind:this={containerRef}
  onscroll={handleScroll}
>
  <div
    style="height: {totalSize}px; width: 100%; position: relative;"
  >
    {#each virtualRows as row (row.key)}
      {@const group = groups[row.index]}
      <div
        style="position: absolute; top: 0; left: 0; width: 100%; height: {row.size}px; transform: translateY({row.start}px);"
      >
        {#if group}
          {@const primary = group.sessions.find(
            (s) => s.id === group.primarySessionId,
          ) ?? group.sessions[0]}
          {#if primary}
            <SessionItem
              session={primary}
              continuationCount={group.sessions.length}
              groupSessionIds={group.sessions.length > 1
                ? group.sessions.map((s) => s.id)
                : undefined}
            />
          {/if}
        {/if}
      </div>
    {/each}
  </div>
</div>

<style>
  .session-list-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 8px 14px;
    font-size: 10px;
    color: var(--text-muted);
    border-bottom: 1px solid var(--border-muted);
    flex-shrink: 0;
    letter-spacing: 0.02em;
    text-transform: uppercase;
  }

  .session-count {
    font-weight: 600;
  }

  .header-actions {
    display: flex;
    align-items: center;
    gap: 8px;
    position: relative;
  }

  .loading-indicator {
    color: var(--accent-green);
  }

  .filter-btn {
    position: relative;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 24px;
    height: 24px;
    border-radius: 4px;
    color: var(--text-muted);
    transition: color 0.1s, background 0.1s;
  }

  .filter-btn:hover {
    color: var(--text-primary);
    background: var(--bg-surface-hover);
  }

  .filter-indicator {
    position: absolute;
    top: 2px;
    right: 2px;
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--accent-green);
  }

  .filter-dropdown {
    position: absolute;
    top: 100%;
    right: 0;
    margin-top: 4px;
    width: 200px;
    background: var(--bg-surface);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-lg);
    padding: 8px;
    z-index: 100;
    text-transform: none;
    letter-spacing: normal;
    animation: dropdown-in 0.12s ease-out;
    transform-origin: top right;
  }

  @keyframes dropdown-in {
    from {
      opacity: 0;
      transform: scale(0.95) translateY(-2px);
    }
    to {
      opacity: 1;
      transform: scale(1) translateY(0);
    }
  }

  .filter-section {
    padding: 4px 0;
  }

  .filter-section + .filter-section {
    border-top: 1px solid var(--border-muted);
    margin-top: 4px;
    padding-top: 8px;
  }

  .filter-section-label {
    font-size: 9px;
    font-weight: 600;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    margin-bottom: 6px;
  }

  .filter-toggle {
    display: flex;
    align-items: center;
    gap: 6px;
    width: 100%;
    padding: 4px 8px;
    font-size: 11px;
    color: var(--text-secondary);
    text-align: left;
    border-radius: 4px;
    transition: background 0.1s, color 0.1s;
  }

  .filter-toggle:hover {
    background: var(--bg-surface-hover);
  }

  .filter-toggle.active {
    background: var(--bg-surface-hover);
    color: var(--accent-green);
    font-weight: 500;
  }

  .toggle-check {
    width: 10px;
    height: 10px;
    border-radius: 2px;
    border: 1.5px solid var(--border-default);
    flex-shrink: 0;
    transition: background 0.1s, border-color 0.1s;
  }

  .toggle-check.on {
    background: var(--accent-green);
    border-color: var(--accent-green);
  }

  .agent-buttons {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
  }

  .agent-filter-btn {
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 2px 8px;
    font-size: 10px;
    color: var(--text-secondary);
    border: 1px solid var(--border-muted);
    border-radius: 4px;
    transition:
      background 0.1s,
      border-color 0.1s,
      color 0.1s;
  }

  .agent-filter-btn:hover {
    background: var(--bg-surface-hover);
    border-color: var(--border-default);
  }

  .agent-filter-btn.active {
    background: var(--bg-surface-hover);
    border-color: var(--agent-color);
    color: var(--agent-color);
    font-weight: 500;
  }

  .agent-dot-mini {
    width: 4px;
    height: 4px;
    border-radius: 50%;
    flex-shrink: 0;
  }

  .clear-filters-btn {
    display: block;
    width: 100%;
    padding: 4px 8px;
    margin-top: 8px;
    font-size: 10px;
    color: var(--text-muted);
    text-align: center;
    border-top: 1px solid var(--border-muted);
    padding-top: 8px;
    transition: color 0.1s;
  }

  .clear-filters-btn:hover {
    color: var(--text-primary);
  }

  .session-list-scroll {
    flex: 1;
    overflow-y: auto;
    overflow-x: hidden;
  }
</style>
