type Theme = "light" | "dark";
type ModalType =
  | "commandPalette"
  | "shortcuts"
  | "publish"
  | null;

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
  showThinking: boolean = $state(true);
  sortNewestFirst: boolean = $state(false);
  activeModal: ModalType = $state(null);
  selectedOrdinal: number | null = $state(null);
  pendingScrollOrdinal: number | null = $state(null);
  pendingScrollSession: string | null = $state(null);

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
  }

  toggleTheme() {
    this.theme = this.theme === "light" ? "dark" : "light";
  }

  toggleThinking() {
    this.showThinking = !this.showThinking;
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
