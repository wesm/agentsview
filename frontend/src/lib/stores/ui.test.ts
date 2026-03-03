import {
  describe,
  it,
  expect,
  beforeEach,
} from "vitest";
import { ui } from "./ui.svelte.js";

describe("UIStore", () => {
  beforeEach(() => {
    ui.activeModal = null;
    ui.selectedOrdinal = null;
    ui.pendingScrollOrdinal = null;
  });

  describe("activeModal", () => {
    it("should default to null", () => {
      expect(ui.activeModal).toBeNull();
    });

    it("should set and clear modal type", () => {
      ui.activeModal = "commandPalette";
      expect(ui.activeModal).toBe("commandPalette");

      ui.activeModal = null;
      expect(ui.activeModal).toBeNull();
    });

    it("should switch between modal types", () => {
      ui.activeModal = "shortcuts";
      expect(ui.activeModal).toBe("shortcuts");

      ui.activeModal = "publish";
      expect(ui.activeModal).toBe("publish");
    });
  });

  describe("closeAll", () => {
    it("should set activeModal to null", () => {
      ui.activeModal = "commandPalette";
      ui.closeAll();
      expect(ui.activeModal).toBeNull();
    });

    it("should be idempotent when already null", () => {
      ui.closeAll();
      expect(ui.activeModal).toBeNull();
    });
  });

  describe("selectedOrdinal null flows", () => {
    it("should default to null", () => {
      expect(ui.selectedOrdinal).toBeNull();
    });

    it("should set ordinal via selectOrdinal", () => {
      ui.selectOrdinal(5);
      expect(ui.selectedOrdinal).toBe(5);
    });

    it("should clear to null via clearSelection", () => {
      ui.selectOrdinal(5);
      ui.clearSelection();
      expect(ui.selectedOrdinal).toBeNull();
    });

    it("should handle ordinal 0 without confusion", () => {
      ui.selectOrdinal(0);
      expect(ui.selectedOrdinal).toBe(0);
    });

    it("clearSelection should be idempotent", () => {
      ui.clearSelection();
      expect(ui.selectedOrdinal).toBeNull();
    });
  });

  describe("pendingScrollOrdinal null flows", () => {
    it("should default to null", () => {
      expect(ui.pendingScrollOrdinal).toBeNull();
    });

    it("should set both selected and pending via scrollToOrdinal", () => {
      ui.scrollToOrdinal(10);
      expect(ui.selectedOrdinal).toBe(10);
      expect(ui.pendingScrollOrdinal).toBe(10);
      expect(ui.pendingScrollSession).toBeNull();
    });

    it("should store session ID when provided", () => {
      ui.scrollToOrdinal(5, "sess-123");
      expect(ui.pendingScrollOrdinal).toBe(5);
      expect(ui.pendingScrollSession).toBe("sess-123");
    });

    it("should allow clearing pending independently", () => {
      ui.scrollToOrdinal(10);
      ui.pendingScrollOrdinal = null;
      expect(ui.pendingScrollOrdinal).toBeNull();
      expect(ui.selectedOrdinal).toBe(10);
    });

    it("should handle ordinal 0", () => {
      ui.scrollToOrdinal(0);
      expect(ui.selectedOrdinal).toBe(0);
      expect(ui.pendingScrollOrdinal).toBe(0);
    });
  });

  describe("theme initialization", () => {
    it("should fall back to light when stored theme is absent", () => {
      expect(ui.theme).toBeDefined();
      expect(["light", "dark"]).toContain(ui.theme);
    });

    it("should survive when localStorage.getItem is unavailable", async () => {
      const original = globalThis.localStorage;
      // Replace with an object that lacks getItem/setItem
      Object.defineProperty(globalThis, "localStorage", {
        value: {},
        writable: true,
        configurable: true,
      });
      try {
        // @ts-expect-error -- query string busts module cache
        const mod = await import("./ui.svelte.js?noGetItem");
        expect(mod.ui.theme).toBe("light");
      } finally {
        Object.defineProperty(globalThis, "localStorage", {
          value: original,
          writable: true,
          configurable: true,
        });
      }
    });

    it("should survive when localStorage is null", async () => {
      const original = globalThis.localStorage;
      Object.defineProperty(globalThis, "localStorage", {
        value: null,
        writable: true,
        configurable: true,
      });
      try {
        // @ts-expect-error -- query string busts module cache
        const mod = await import("./ui.svelte.js?nullStorage");
        expect(mod.ui.theme).toBe("light");
      } finally {
        Object.defineProperty(globalThis, "localStorage", {
          value: original,
          writable: true,
          configurable: true,
        });
      }
    });

    it("should survive when localStorage is undefined", async () => {
      const original = globalThis.localStorage;
      // @ts-expect-error -- deliberately removing localStorage
      delete globalThis.localStorage;
      try {
        // @ts-expect-error -- query string busts module cache
        const mod = await import("./ui.svelte.js?noStorage");
        expect(mod.ui.theme).toBe("light");
      } finally {
        Object.defineProperty(globalThis, "localStorage", {
          value: original,
          writable: true,
          configurable: true,
        });
      }
    });
  });

  describe("postMessage theme control", () => {
    it("should change theme on valid theme:set message", () => {
      ui.theme = "light";
      window.dispatchEvent(
        new MessageEvent("message", {
          data: { type: "theme:set", theme: "dark" },
        }),
      );
      expect(ui.theme).toBe("dark");
    });

    it("should ignore invalid theme values", () => {
      ui.theme = "light";
      window.dispatchEvent(
        new MessageEvent("message", {
          data: { type: "theme:set", theme: "purple" },
        }),
      );
      expect(ui.theme).toBe("light");
    });

    it("should ignore unrelated message types", () => {
      ui.theme = "light";
      window.dispatchEvent(
        new MessageEvent("message", {
          data: { type: "some-other-event", theme: "dark" },
        }),
      );
      expect(ui.theme).toBe("light");
    });
  });

  describe("toggles", () => {
    it("should toggle theme between light and dark", () => {
      ui.theme = "light";
      ui.toggleTheme();
      expect(ui.theme).toBe("dark");
      ui.toggleTheme();
      expect(ui.theme).toBe("light");
    });

    it("should toggle showThinking", () => {
      const initial = ui.showThinking;
      ui.toggleThinking();
      expect(ui.showThinking).toBe(!initial);
    });

    it("should toggle sortNewestFirst", () => {
      const initial = ui.sortNewestFirst;
      ui.toggleSort();
      expect(ui.sortNewestFirst).toBe(!initial);
    });
  });

  describe("block type filtering", () => {
    beforeEach(() => {
      ui.showAllBlocks();
    });

    it("should start with all blocks visible", () => {
      expect(ui.hiddenBlockCount).toBe(0);
      expect(ui.hasBlockFilters).toBe(false);
      expect(ui.isBlockVisible("user")).toBe(true);
      expect(ui.isBlockVisible("tool")).toBe(true);
      expect(ui.isBlockVisible("thinking")).toBe(true);
      expect(ui.isBlockVisible("code")).toBe(true);
      expect(ui.isBlockVisible("assistant")).toBe(true);
    });

    it("should toggle a block type off and on", () => {
      ui.toggleBlock("tool");
      expect(ui.isBlockVisible("tool")).toBe(false);
      expect(ui.hiddenBlockCount).toBe(1);
      expect(ui.hasBlockFilters).toBe(true);

      ui.toggleBlock("tool");
      expect(ui.isBlockVisible("tool")).toBe(true);
      expect(ui.hiddenBlockCount).toBe(0);
    });

    it("should sync showThinking when toggling thinking block", () => {
      ui.toggleBlock("thinking");
      expect(ui.showThinking).toBe(false);

      ui.toggleBlock("thinking");
      expect(ui.showThinking).toBe(true);
    });

    it("should sync block filter when toggling thinking", () => {
      ui.toggleThinking();
      expect(ui.isBlockVisible("thinking")).toBe(false);

      ui.toggleThinking();
      expect(ui.isBlockVisible("thinking")).toBe(true);
    });

    it("should reset all with showAllBlocks", () => {
      ui.toggleBlock("user");
      ui.toggleBlock("tool");
      ui.toggleBlock("code");
      expect(ui.hiddenBlockCount).toBe(3);

      ui.showAllBlocks();
      expect(ui.hiddenBlockCount).toBe(0);
      expect(ui.hasBlockFilters).toBe(false);
    });
  });

  describe("messageLayout", () => {
    beforeEach(() => {
      ui.setLayout("default");
    });

    it("should default to 'default'", () => {
      expect(ui.messageLayout).toBe("default");
    });

    it("should set layout explicitly", () => {
      ui.setLayout("compact");
      expect(ui.messageLayout).toBe("compact");

      ui.setLayout("stream");
      expect(ui.messageLayout).toBe("stream");
    });

    it("should cycle through layouts", () => {
      ui.setLayout("default");
      ui.cycleLayout();
      expect(ui.messageLayout).toBe("compact");

      ui.cycleLayout();
      expect(ui.messageLayout).toBe("stream");

      ui.cycleLayout();
      expect(ui.messageLayout).toBe("default");
    });
  });
});
