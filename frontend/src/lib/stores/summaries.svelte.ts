import type {
  Summary,
  SummaryType,
  AgentName,
} from "../api/types.js";
import {
  listSummaries,
  generateSummary,
  type GenerateSummaryHandle,
} from "../api/client.js";

function localDateStr(d: Date): string {
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

class SummariesStore {
  date: string = $state(localDateStr(new Date()));
  type: SummaryType = $state("daily_activity");
  project: string = $state("");
  agent: AgentName = $state("claude");
  summaries: Summary[] = $state([]);
  selectedId: number | null = $state(null);
  loading = $state(false);
  generating = $state(false);
  generatePhase: string = $state("");
  generateError: string | null = $state(null);
  promptText: string = $state("");

  #handle: GenerateSummaryHandle | null = null;
  #version = 0;

  get selectedSummary(): Summary | undefined {
    return this.summaries.find(
      (s) => s.id === this.selectedId,
    );
  }

  async load() {
    const v = ++this.#version;
    this.loading = true;
    try {
      const res = await listSummaries({
        type: this.type,
        date: this.date,
        project: this.project || undefined,
      });
      if (this.#version === v) {
        this.summaries = res.summaries;
        if (
          this.selectedId !== null &&
          !this.summaries.some(
            (s) => s.id === this.selectedId,
          )
        ) {
          this.selectedId = null;
        }
      }
    } catch {
      if (this.#version === v) {
        this.summaries = [];
      }
    } finally {
      if (this.#version === v) {
        this.loading = false;
      }
    }
  }

  setDate(date: string) {
    this.date = date;
    this.selectedId = null;
    this.load();
  }

  setType(type: SummaryType) {
    this.type = type;
    this.selectedId = null;
    this.load();
  }

  setProject(project: string) {
    this.project = project;
    this.selectedId = null;
    this.load();
  }

  setAgent(agent: AgentName) {
    this.agent = agent;
  }

  select(id: number) {
    this.selectedId = id;
  }

  async generate() {
    this.generating = true;
    this.generatePhase = "generating";
    this.generateError = null;

    const snap = {
      type: this.type,
      date: this.date,
      project: this.project,
    };

    this.#handle = generateSummary(
      {
        type: snap.type,
        date: snap.date,
        project: snap.project || undefined,
        prompt: this.promptText || undefined,
        agent: this.agent,
      },
      (phase) => {
        this.generatePhase = phase;
      },
    );

    try {
      const summary = await this.#handle.done;
      const filtersMatch =
        this.type === snap.type &&
        this.date === snap.date &&
        this.project === snap.project;
      if (filtersMatch) {
        this.summaries = [summary, ...this.summaries];
        this.selectedId = summary.id;
      } else {
        this.load();
      }
    } catch (e) {
      if (e instanceof DOMException && e.name === "AbortError") {
        // User cancelled
      } else {
        this.generateError =
          e instanceof Error
            ? e.message
            : "Generation failed";
      }
    } finally {
      this.generating = false;
      this.generatePhase = "";
      this.#handle = null;
    }
  }

  cancelGeneration() {
    this.#handle?.abort();
  }
}

export const summaries = new SummariesStore();
