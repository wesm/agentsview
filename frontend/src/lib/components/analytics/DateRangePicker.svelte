<script lang="ts">
  import { analytics } from "../../stores/analytics.svelte.js";

  interface Preset {
    label: string;
    days: number;
  }

  const presets: Preset[] = [
    { label: "7d", days: 7 },
    { label: "30d", days: 30 },
    { label: "90d", days: 90 },
    { label: "1y", days: 365 },
    { label: "All", days: 0 },
  ];

  function localDateStr(d: Date): string {
    const y = d.getFullYear();
    const m = String(d.getMonth() + 1).padStart(2, "0");
    const day = String(d.getDate()).padStart(2, "0");
    return `${y}-${m}-${day}`;
  }

  function daysAgo(n: number): string {
    const d = new Date();
    d.setDate(d.getDate() - n);
    return localDateStr(d);
  }

  function todayStr(): string {
    return localDateStr(new Date());
  }

  const ALL_FROM = "1970-01-01";

  function applyPreset(days: number) {
    if (days === 0) {
      analytics.setDateRange(ALL_FROM, todayStr());
    } else {
      analytics.setDateRange(daysAgo(days), todayStr());
    }
  }

  function isActive(days: number): boolean {
    const from = days === 0 ? ALL_FROM : daysAgo(days);
    return analytics.from === from &&
      analytics.to === todayStr();
  }
</script>

<div class="date-range-picker">
  <div class="presets">
    {#each presets as preset}
      <button
        class="preset-btn"
        class:active={isActive(preset.days)}
        onclick={() => applyPreset(preset.days)}
      >
        {preset.label}
      </button>
    {/each}
  </div>

  <div class="date-inputs">
    <input
      type="date"
      class="date-input"
      value={analytics.from}
      onchange={(e) => {
        const target = e.target as HTMLInputElement;
        analytics.setDateRange(target.value, analytics.to);
      }}
    />
    <span class="date-sep">-</span>
    <input
      type="date"
      class="date-input"
      value={analytics.to}
      onchange={(e) => {
        const target = e.target as HTMLInputElement;
        analytics.setDateRange(analytics.from, target.value);
      }}
    />
  </div>
</div>

<style>
  .date-range-picker {
    display: flex;
    align-items: center;
    gap: 12px;
  }

  .presets {
    display: flex;
    gap: 2px;
  }

  .preset-btn {
    height: 24px;
    padding: 0 8px;
    border-radius: var(--radius-sm);
    font-size: 11px;
    font-weight: 500;
    color: var(--text-muted);
    cursor: pointer;
    transition: background 0.1s, color 0.1s;
  }

  .preset-btn:hover {
    background: var(--bg-surface-hover);
    color: var(--text-secondary);
  }

  .preset-btn.active {
    background: var(--accent-blue);
    color: #fff;
  }

  .date-inputs {
    display: flex;
    align-items: center;
    gap: 4px;
  }

  .date-input {
    height: 24px;
    padding: 0 6px;
    background: var(--bg-inset);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-sm);
    font-size: 11px;
    color: var(--text-secondary);
    font-family: var(--font-mono);
  }

  .date-input:focus {
    outline: none;
    border-color: var(--accent-blue);
  }

  .date-sep {
    color: var(--text-muted);
    font-size: 11px;
  }
</style>
