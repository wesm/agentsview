<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import DateRangePicker from "./DateRangePicker.svelte";
  import SummaryCards from "./SummaryCards.svelte";
  import Heatmap from "./Heatmap.svelte";
  import ActivityTimeline from "./ActivityTimeline.svelte";
  import ProjectBreakdown from "./ProjectBreakdown.svelte";
  import HourOfWeekHeatmap from "./HourOfWeekHeatmap.svelte";
  import SessionShape from "./SessionShape.svelte";
  import VelocityMetrics from "./VelocityMetrics.svelte";
  import ToolUsage from "./ToolUsage.svelte";
  import AgentComparison from "./AgentComparison.svelte";
  import { analytics } from "../../stores/analytics.svelte.js";
  import { exportAnalyticsCSV } from "../../utils/csv-export.js";

  const REFRESH_INTERVAL_MS = 5 * 60 * 1000;

  function handleExportCSV() {
    exportAnalyticsCSV({
      from: analytics.from,
      to: analytics.to,
      summary: analytics.summary,
      activity: analytics.activity,
      projects: analytics.projects,
      tools: analytics.tools,
      velocity: analytics.velocity,
    });
  }

  let refreshTimer: ReturnType<typeof setInterval> | undefined;

  onMount(() => {
    analytics.fetchAll();
    refreshTimer = setInterval(
      () => analytics.fetchAll(),
      REFRESH_INTERVAL_MS,
    );
  });

  onDestroy(() => {
    if (refreshTimer !== undefined) {
      clearInterval(refreshTimer);
    }
  });
</script>

<div class="analytics-page">
  <div class="analytics-toolbar">
    <DateRangePicker />
    <button
      class="refresh-btn"
      onclick={() => analytics.fetchAll()}
      title="Refresh analytics"
      aria-label="Refresh analytics"
    >
      <svg width="14" height="14" viewBox="0 0 16 16" fill="currentColor">
        <path d="M8 3a5 5 0 00-4.546 2.914.5.5 0 01-.908-.418A6 6 0 0114 8a.5.5 0 01-1 0 5 5 0 00-5-5zm4.546 7.086a.5.5 0 01.908.418A6 6 0 012 8a.5.5 0 011 0 5 5 0 005 5 5 5 0 004.546-2.914z"/>
      </svg>
    </button>
    <button class="export-btn" onclick={handleExportCSV}>
      Export CSV
    </button>
  </div>

  <div class="analytics-content">
    <SummaryCards />

    <div class="chart-grid">
      <div class="chart-panel wide">
        <Heatmap />
      </div>

      <div class="chart-panel wide">
        <HourOfWeekHeatmap />
      </div>

      <div class="chart-panel">
        <ActivityTimeline />
      </div>

      <div class="chart-panel">
        <ProjectBreakdown />
      </div>

      <div class="chart-panel">
        <SessionShape />
      </div>

      <div class="chart-panel wide">
        <VelocityMetrics />
      </div>

      <div class="chart-panel wide">
        <ToolUsage />
      </div>

      <div class="chart-panel wide">
        <AgentComparison />
      </div>
    </div>
  </div>
</div>

<style>
  .analytics-page {
    flex: 1;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }

  .analytics-toolbar {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 8px 16px;
    background: var(--bg-surface);
    border-bottom: 1px solid var(--border-muted);
    flex-shrink: 0;
  }

  .refresh-btn {
    width: 28px;
    height: 28px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: var(--radius-sm);
    color: var(--text-muted);
    cursor: pointer;
  }

  .refresh-btn:hover {
    background: var(--bg-surface-hover);
    color: var(--text-primary);
  }

  .export-btn {
    height: 24px;
    padding: 0 8px;
    border-radius: var(--radius-sm);
    font-size: 11px;
    font-weight: 500;
    color: var(--text-muted);
    cursor: pointer;
    transition: background 0.1s, color 0.1s;
    margin-left: auto;
  }

  .export-btn:hover {
    background: var(--bg-surface-hover);
    color: var(--text-secondary);
  }

  .analytics-content {
    flex: 1;
    overflow-y: auto;
    padding: 16px;
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  .chart-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px;
  }

  .chart-panel {
    background: var(--bg-surface);
    border: 1px solid var(--border-muted);
    border-radius: var(--radius-md);
    padding: 12px;
    min-height: 200px;
    min-width: 0;
    overflow: hidden;
    display: flex;
    flex-direction: column;
  }

  .chart-panel.wide {
    grid-column: 1 / -1;
  }

  @media (max-width: 800px) {
    .chart-grid {
      grid-template-columns: 1fr;
    }
  }
</style>
