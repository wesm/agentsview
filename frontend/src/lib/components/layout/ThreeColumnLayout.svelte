<script lang="ts">
  import type { Snippet } from "svelte";

  interface Props {
    sidebar: Snippet;
    content: Snippet;
  }

  let { sidebar, content }: Props = $props();
</script>

<div class="layout">
  <aside class="sidebar">
    {@render sidebar()}
  </aside>
  <main class="content">
    {@render content()}
  </main>
</div>

<style>
  .layout {
    display: flex;
    height: calc(100vh - 40px - 24px);
    overflow: hidden;
  }

  .sidebar {
    width: 260px;
    flex-shrink: 0;
    border-right: 1px solid var(--border-default);
    overflow: hidden;
    display: flex;
    flex-direction: column;
  }

  .content {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    display: flex;
    flex-direction: column;
  }
</style>
