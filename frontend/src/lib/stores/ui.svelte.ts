type Theme = "light" | "dark";
export type MessageLayout = "default" | "compact" | "stream";
type ModalType =
  | "commandPalette"
  | "shortcuts"
  | "publish"
  | "resync"
  | "update"
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

const LAYOUT_KEY = "agentsview-message-layout";
const VALID_LAYOUTS: MessageLayout[] = [
  "default",
  "compact",
  "stream",
];
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

function readStoredLayout(): MessageLayout {
  try {
    const raw = localStorage?.getItem(LAYOUT_KEY);
    if (
      raw &&
      VALID_LAYOUTS.includes(raw as MessageLayout)
    ) {
      return raw as MessageLayout;
    }
  } catch {
    // ignore
  }
  return "default";
}

class UIStore {
  theme: Theme = $state(readStoredTheme() || "light");
  sortNewestFirst: boolean = $state(false);
  messageLayout: MessageLayout = $state(readStoredLayout());
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

      $effect(() => {
        try {
          localStorage?.setItem(
            LAYOUT_KEY,
            this.messageLayout,
          );
        } catch {
          // ignore
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
    this.persistBlockFilters();
  }

  showAllBlocks() {
    this.visibleBlocks = new Set(ALL_BLOCK_TYPES);
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

  cycleLayout() {
    const idx = VALID_LAYOUTS.indexOf(this.messageLayout);
    this.messageLayout =
      VALID_LAYOUTS[(idx + 1) % VALID_LAYOUTS.length]!;
  }

  setLayout(layout: MessageLayout) {
    this.messageLayout = layout;
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
