import * as api from "../api/client.js";
import { debounce } from "../utils/debounce.js";
import type { SearchResult } from "../api/types.js";

class SearchStore {
  query: string = $state("");
  project: string = $state("");
  results: SearchResult[] = $state([]);
  isSearching: boolean = $state(false);

  private abortController: AbortController | null = null;

  private debouncedSearch = debounce(
    (q: string, project: string) => {
      this.executeSearch(q, project);
    },
    300,
  );

  search(q: string, project?: string) {
    this.query = q;
    if (project !== undefined) this.project = project;

    if (!q.trim()) {
      this.debouncedSearch.cancel();
      this.abortController?.abort();
      this.results = [];
      this.isSearching = false;
      return;
    }

    this.abortController?.abort();
    this.abortController = null;
    this.debouncedSearch(q, this.project);
  }

  clear() {
    this.query = "";
    this.results = [];
    this.isSearching = false;
    this.debouncedSearch.cancel();
    this.abortController?.abort();
  }

  private async executeSearch(
    q: string, project: string,
  ) {
    this.abortController?.abort();
    this.abortController = new AbortController();
    const { signal } = this.abortController;

    this.isSearching = true;
    try {
      const res = await api.search(
        q,
        { project: project || undefined, limit: 30 },
        { signal },
      );
      this.results = res.results;
    } catch (error: unknown) {
      if (error instanceof DOMException
        && error.name === "AbortError") {
        return;
      }
      this.results = [];
    } finally {
      if (!signal.aborted) {
        this.isSearching = false;
      }
    }
  }
}

export const searchStore = new SearchStore();
