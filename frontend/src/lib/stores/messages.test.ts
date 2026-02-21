import { describe, it, expect, vi, beforeEach } from 'vitest';
import { messages } from './messages.svelte.js';
import * as api from '../api/client.js';
import type {
  Message,
  MessagesResponse,
  MinimapResponse,
  Session,
} from '../api/types.js';

// Helper to wait for a condition
async function waitFor(callback: () => void | Promise<void>, timeout = 1000) {
  const start = Date.now();
  while (true) {
    try {
      await callback();
      return;
    } catch (e) {
      if (Date.now() - start > timeout) {
        throw e;
      }
      await new Promise(resolve => setTimeout(resolve, 10));
    }
  }
}

// Mock the API client
vi.mock('../api/client.js', () => ({
  getMessages: vi.fn(),
  getMinimap: vi.fn(),
  getSession: vi.fn(),
}));

function makeSession(
  id: string,
  messageCount: number,
): Session {
  return {
    id,
    project: 'project-alpha',
    machine: 'test-machine',
    agent: 'test-agent',
    first_message: null,
    started_at: null,
    ended_at: null,
    message_count: messageCount,
    created_at: new Date(0).toISOString(),
  };
}

function makeMessage(ordinal: number): Message {
  return {
    id: ordinal + 1,
    session_id: 's1',
    ordinal,
    role: ordinal % 2 === 0 ? 'user' : 'assistant',
    content: `msg ${ordinal}`,
    timestamp: new Date(ordinal * 1000).toISOString(),
    has_thinking: false,
    has_tool_use: false,
    content_length: 6,
  };
}

function makeMessagesResponse(
  rows: Message[],
): MessagesResponse {
  return {
    messages: rows,
    count: rows.length,
  };
}

function emptyMinimap(): MinimapResponse {
  return {
    entries: [],
    count: 0,
  };
}

describe('MessagesStore', () => {
  beforeEach(() => {
    messages.clear();
    vi.clearAllMocks();
  });

  it('should clear reload state when loading a new session', async () => {
    // Setup initial session s1
    vi.mocked(api.getSession).mockResolvedValue(
      makeSession('s1', 10),
    );
    vi.mocked(api.getMessages).mockResolvedValue(
      makeMessagesResponse([]),
    );
    vi.mocked(api.getMinimap).mockResolvedValue(emptyMinimap());

    await messages.loadSession('s1');
    expect(messages.sessionId).toBe('s1');

    // Trigger a reload that hangs
    let resolveReload!: (value: Session) => void;
    const pendingReload = new Promise<Session>((resolve) => {
      resolveReload = resolve;
    });
    vi.mocked(api.getSession).mockReturnValue(pendingReload);

    const p1 = messages.reload();
    
    // Check internal state (using any to access private fields)
    expect((messages as any).reloadPromise).toBe(p1);
    expect((messages as any).reloadSessionId).toBe('s1');

    // Switch to session s2
    vi.mocked(api.getSession).mockResolvedValue({ id: 's2', message_count: 5 } as any);
    await messages.loadSession('s2');

    // Verify reload state is cleared
    expect(messages.sessionId).toBe('s2');
    expect((messages as any).reloadPromise).toBeNull();
    expect((messages as any).reloadSessionId).toBeNull();

    // Resolve the dangling promise to clean up
    resolveReload(makeSession('s1', 10));
    await p1;
  });

  it('should not reuse reload promise from different session', async () => {
    // Setup session s1
    vi.mocked(api.getSession).mockResolvedValue(
      makeSession('s1', 10),
    );
    vi.mocked(api.getMessages).mockResolvedValue(
      makeMessagesResponse([]),
    );
    vi.mocked(api.getMinimap).mockResolvedValue(emptyMinimap());

    await messages.loadSession('s1');

    // Start reload for s1
    let resolveS1!: (value: Session) => void;
    const s1Promise = new Promise<Session>((resolve) => {
      resolveS1 = resolve;
    });
    vi.mocked(api.getSession).mockReturnValue(s1Promise);

    const p1 = messages.reload();
    expect((messages as any).reloadSessionId).toBe('s1');

    // Simulate switching to s2 WITHOUT going through loadSession (e.g. race condition or direct ID manipulation if possible, 
    // but here we simulate the state where we are on s2 but p1 is still in flight).
    // Actually, properly we should use loadSession.
    vi.mocked(api.getSession).mockResolvedValue(
      makeSession('s2', 5),
    );
    await messages.loadSession('s2');

    // Now on s2. p1 is still running (conceptually).
    // Start reload for s2.
    // We need to mock getSession for s2 reload.
    let resolveS2!: (value: Session) => void;
    const s2Promise = new Promise<Session>((resolve) => {
      resolveS2 = resolve;
    });
    vi.mocked(api.getSession).mockReturnValue(s2Promise);

    const p2 = messages.reload();
    
    expect((messages as any).reloadSessionId).toBe('s2');
    expect(p2).not.toBe(p1);

    resolveS1(makeSession('s1', 10));
    resolveS2(makeSession('s2', 5));
    await Promise.all([p1, p2]);
  });
  
  it('should coalesce reloads for the same session', async () => {
    // Setup session s1
    vi.mocked(api.getSession).mockResolvedValue(
      makeSession('s1', 10),
    );
    vi.mocked(api.getMessages).mockResolvedValue(
      makeMessagesResponse([]),
    );
    vi.mocked(api.getMinimap).mockResolvedValue(emptyMinimap());

    await messages.loadSession('s1');

    // Start reload
    let resolveS1!: (value: Session) => void;
    const s1Promise = new Promise<Session>((resolve) => {
      resolveS1 = resolve;
    });
    vi.mocked(api.getSession)
      .mockReturnValueOnce(s1Promise)
      .mockResolvedValue(makeSession('s1', 10));

    const p1 = messages.reload();
    const p2 = messages.reload();

    expect(p1).toBe(p2);
    expect((messages as any).pendingReload).toBe(true);
    
    resolveS1(makeSession('s1', 10));
    await p1;
  });

  it('should no-op ensureOrdinalLoaded when full session is already loaded', async () => {
    // Setup session s1
    vi.mocked(api.getSession).mockResolvedValue(
      makeSession('s1', 20),
    );
    vi.mocked(api.getMinimap).mockResolvedValue(emptyMinimap());
    
    // Full load returns all messages in ascending order.
    vi.mocked(api.getMessages).mockResolvedValueOnce(
      makeMessagesResponse(
        Array.from(
          { length: 20 },
          (_, i) => makeMessage(i),
        ),
      ),
    );

    await messages.loadSession('s1');
    
    expect(messages.messages.length).toBe(20);
    expect(messages.messages[0]).toBeDefined();
    expect(messages.messages[0]!.ordinal).toBe(0);
    expect(messages.hasOlder).toBe(false);
    
    await messages.ensureOrdinalLoaded(5);
    
    expect(vi.mocked(api.getMessages)).toHaveBeenCalledTimes(1);
    expect(messages.messages.length).toBe(20);
    expect(messages.messages[0]).toBeDefined();
    expect(messages.messages[0]!.ordinal).toBe(0);
  });

  it('should not clear pending reload of a new session when old session reload finishes', async () => {
    // 1. Setup Session A
    vi.mocked(api.getSession).mockResolvedValue(makeSession('s1', 10));
    vi.mocked(api.getMessages).mockResolvedValue(makeMessagesResponse([]));
    vi.mocked(api.getMinimap).mockResolvedValue(emptyMinimap());
    await messages.loadSession('s1');

    // 2. Start Reload for Session A (P1) - hangs
    let resolveP1!: (value: Session) => void;
    const p1Promise = new Promise<Session>((resolve) => { resolveP1 = resolve; });
    vi.mocked(api.getSession).mockReturnValue(p1Promise as any); 
    
    const p1 = messages.reload();

    // 3. Switch to Session B
    vi.mocked(api.getSession).mockResolvedValue(makeSession('s2', 5));
    await messages.loadSession('s2');

    // 4. Start Reload for Session B (P2) - hangs
    let resolveP2!: (value: Session) => void;
    const p2Promise = new Promise<Session>((resolve) => { resolveP2 = resolve; });
    vi.mocked(api.getSession).mockReturnValue(p2Promise as any);

    const p2 = messages.reload();

    // 5. Trigger Pending Reload for Session B (sets pendingReload = true)
    const p3 = messages.reload();
    expect((messages as any).pendingReload).toBe(true);
    expect(p3).toBe(p2); // Should reuse P2

    // 6. Resolve P1 (Session A). 
    // This should NOT clear pendingReload, because P1 belongs to S1, and current is S2.
    resolveP1(makeSession('s1', 10));
    
    // Wait a tick for P1 finally block
    await new Promise(resolve => setTimeout(resolve, 0));

    // CHECK: pendingReload should still be true (If bug exists, this might fail or be flaky if we didn't wait enough, but logically it fails because it IS cleared)
    expect((messages as any).pendingReload).toBe(true);

    // 7. Resolve P2 (Session B).
    // This should trigger the pending reload.
    // Mock the NEXT getSession call for the *pending* reload which will execute now.
    vi.mocked(api.getSession).mockResolvedValue(makeSession('s2', 6)); 
    resolveP2(makeSession('s2', 5));

    await p2;
    
    // Check that pendingReload is now false (consumed)
    expect((messages as any).pendingReload).toBe(false);
  });

  it('should fallback to full reload if incremental fetch is out of sync', async () => {
    // 1. Initial State: Session 's1' with 2 messages
    vi.mocked(api.getSession).mockResolvedValue(makeSession('s1', 2));
    vi.mocked(api.getMinimap).mockResolvedValue(emptyMinimap());
    vi.mocked(api.getMessages).mockResolvedValueOnce(
      makeMessagesResponse([makeMessage(0), makeMessage(1)])
    );

    await messages.loadSession('s1');
    expect(messages.messageCount).toBe(2);

    // 2. Prepare for Reload
    // New state on server: count=4.
    // Incremental fetch returns only [2], missing [3].
    // This mismatch (newest=2 vs expected=3) should trigger full reload.

    // (A) getSession (reloadNow start)
    vi.mocked(api.getSession).mockResolvedValueOnce(makeSession('s1', 4));

    // (B) getMessages (loadFrom - incremental)
    vi.mocked(api.getMessages).mockResolvedValueOnce(
      makeMessagesResponse([makeMessage(2)])
    );

    // (C) getMessages (fullReload -> loadAllMessages)
    vi.mocked(api.getMessages).mockResolvedValueOnce(
      makeMessagesResponse([
        makeMessage(1),
        makeMessage(0),
        makeMessage(2),
        makeMessage(3),
      ])
    );

    await messages.reload();

    expect(messages.messageCount).toBe(4);
    expect(messages.messages.length).toBe(4);
    expect(messages.messages[3]!.ordinal).toBe(3);

    // Verify full reload fetched all messages
    expect(vi.mocked(api.getMessages)).toHaveBeenLastCalledWith(
      's1',
      expect.objectContaining({ from: 0, limit: 1000, direction: 'asc' }),
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    );
  });

  it('should not update messageCount prematurely if incremental fetch fails and triggers full reload', async () => {
    // 1. Initial State: Session 's1' with 2 messages
    vi.mocked(api.getSession).mockResolvedValue(makeSession('s1', 2));
    vi.mocked(api.getMinimap).mockResolvedValue(emptyMinimap());
    vi.mocked(api.getMessages).mockResolvedValueOnce(
      makeMessagesResponse([makeMessage(0), makeMessage(1)])
    );

    await messages.loadSession('s1');
    expect(messages.messageCount).toBe(2);

    // 2. Prepare for Reload
    // New state on server: count=4.
    // Incremental fetch returns only [2] (mismatch with 4, because missing 3).
    // This will trigger full reload.

    // (A) getSession (reloadNow start)
    vi.mocked(api.getSession).mockResolvedValueOnce(makeSession('s1', 4));

    // (B) getMessages (loadFrom - incremental) - returns fast
    vi.mocked(api.getMessages).mockResolvedValueOnce(
      makeMessagesResponse([makeMessage(2)])
    );

    // (C) getMessages (fullReload -> loadAllMessages) - DELAYED
    let resolveFullReload!: (val: MessagesResponse) => void;
    const fullReloadPromise = new Promise<MessagesResponse>(resolve => {
        resolveFullReload = resolve;
    });
    vi.mocked(api.getMessages).mockReturnValueOnce(fullReloadPromise as any);

    // Start reload
    const reloadPromise = messages.reload();

    // Wait for the full reload call to be initiated (Initial + Incremental + Full = 3 calls)
    await waitFor(() => {
        expect(vi.mocked(api.getMessages)).toHaveBeenCalledTimes(3);
    });

    // CHECK: messageCount should still be 2.
    // If bug existed (we fell through `if` instead of returning), messageCount would be 4.
    expect(messages.messageCount).toBe(2);

    // Finish full reload
    resolveFullReload(makeMessagesResponse([
        makeMessage(0), makeMessage(1), makeMessage(2), makeMessage(3)
    ]));

    await reloadPromise;

    // CHECK: messageCount is now 4
    expect(messages.messageCount).toBe(4);
  });
});
