<script lang="ts">
  import { onDestroy } from "svelte";
  import { sessions } from "../../stores/sessions.svelte.js";
  import SessionItem from "./SessionItem.svelte";
  import { formatNumber } from "../../utils/format.js";

  const ITEM_HEIGHT = 40;
  const OVERSCAN = 10;

  let containerRef: HTMLDivElement | undefined = $state(undefined);
  let scrollTop = $state(0);
  let viewportHeight = $state(0);
  let scrollRaf: number | null = $state(null);

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
  {#if sessions.loading}
    <span class="loading-indicator">loading</span>
  {/if}
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
    padding: 8px 12px;
    font-size: 11px;
    color: var(--text-muted);
    border-bottom: 1px solid var(--border-muted);
    flex-shrink: 0;
  }

  .session-count {
    font-weight: 500;
  }

  .loading-indicator {
    color: var(--accent-green);
  }

  .session-list-scroll {
    flex: 1;
    overflow-y: auto;
    overflow-x: hidden;
  }
</style>
