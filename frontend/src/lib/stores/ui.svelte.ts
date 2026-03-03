type Theme = "light" | "dark";
type ModalType =
  | "commandPalette"
  | "shortcuts"
  | "publish"
  | "resync"
  | null;

/** Block types that can be toggled visible/hidden. */
export type BlockType =
  | "user"
  | "assistant"
  | "thinking"
  | "tool"
  | "code";

export const ALL_BLOCK_TYPES: BlockType[] = [
  "user",
  "assistant",
  "thinking",
  "tool",
  "code",
];

const BLOCK_FILTER_KEY = "agentsview-block-filters";

function readBlockFilters(): Set<BlockType> {
  try {
    const raw = localStorage?.getItem(BLOCK_FILTER_KEY);
    if (raw) {
      const arr = JSON.parse(raw);
      if (Array.isArray(arr)) {
        return new Set(
          arr.filter((t: string) =>
            ALL_BLOCK_TYPES.includes(t as BlockType),
          ) as BlockType[],
        );
      }
    }
  } catch {
    // ignore
  }
  return new Set(ALL_BLOCK_TYPES);
}

function readStoredTheme(): Theme | null {
  if (
    typeof localStorage !== "undefined" &&
    localStorage != null &&
    typeof localStorage.getItem === "function"
  ) {
    return localStorage.getItem("theme") as Theme;
  }
  return null;
}

class UIStore {
  theme: Theme = $state(readStoredTheme() || "light");
  showThinking: boolean = $state(readBlockFilters().has("thinking"));
  sortNewestFirst: boolean = $state(false);
  activeModal: ModalType = $state(null);
  selectedOrdinal: number | null = $state(null);
  pendingScrollOrdinal: number | null = $state(null);
  pendingScrollSession: string | null = $state(null);

  /** Set of block types currently visible. */
  visibleBlocks: Set<BlockType> = $state(readBlockFilters());

  constructor() {
    $effect.root(() => {
      $effect(() => {
        const root = document.documentElement;
        if (this.theme === "dark") {
          root.classList.add("dark");
        } else {
          root.classList.remove("dark");
        }
        if (
          typeof localStorage !== "undefined" &&
          localStorage != null &&
          typeof localStorage.setItem === "function"
        ) {
          localStorage.setItem("theme", this.theme);
        }
      });
    });

    // Allow parent windows to control theme via postMessage
    if (typeof window !== "undefined") {
      window.addEventListener("message", (event: MessageEvent) => {
        if (
          event.data &&
          event.data.type === "theme:set" &&
          (event.data.theme === "light" || event.data.theme === "dark")
        ) {
          this.theme = event.data.theme;
        }
      });
    }
  }

  toggleTheme() {
    this.theme = this.theme === "light" ? "dark" : "light";
  }

  toggleThinking() {
    this.showThinking = !this.showThinking;
    // Keep block filter in sync
    const next = new Set(this.visibleBlocks);
    if (this.showThinking) {
      next.add("thinking");
    } else {
      next.delete("thinking");
    }
    this.visibleBlocks = next;
    this.persistBlockFilters();
  }

  isBlockVisible(type: BlockType): boolean {
    return this.visibleBlocks.has(type);
  }

  setBlockVisible(type: BlockType, visible: boolean) {
    const next = new Set(this.visibleBlocks);
    if (visible) {
      next.add(type);
    } else {
      next.delete(type);
    }
    this.visibleBlocks = next;
    this.showThinking = next.has("thinking");
    this.persistBlockFilters();
  }

  toggleBlock(type: BlockType) {
    const next = new Set(this.visibleBlocks);
    if (next.has(type)) {
      next.delete(type);
    } else {
      next.add(type);
    }
    this.visibleBlocks = next;
    // Keep showThinking in sync
    this.showThinking = next.has("thinking");
    this.persistBlockFilters();
  }

  showAllBlocks() {
    this.visibleBlocks = new Set(ALL_BLOCK_TYPES);
    this.showThinking = true;
    this.persistBlockFilters();
  }

  get hiddenBlockCount(): number {
    return ALL_BLOCK_TYPES.length - this.visibleBlocks.size;
  }

  get hasBlockFilters(): boolean {
    return this.visibleBlocks.size < ALL_BLOCK_TYPES.length;
  }

  private persistBlockFilters() {
    try {
      localStorage?.setItem(
        BLOCK_FILTER_KEY,
        JSON.stringify([...this.visibleBlocks]),
      );
    } catch {
      // ignore
    }
  }

  toggleSort() {
    this.sortNewestFirst = !this.sortNewestFirst;
  }

  selectOrdinal(ordinal: number) {
    this.selectedOrdinal = ordinal;
  }

  clearSelection() {
    this.selectedOrdinal = null;
  }

  scrollToOrdinal(ordinal: number, sessionId?: string) {
    this.selectedOrdinal = ordinal;
    this.pendingScrollOrdinal = ordinal;
    this.pendingScrollSession = sessionId ?? null;
  }

  closeAll() {
    this.activeModal = null;
  }
}

export const ui = new UIStore();
