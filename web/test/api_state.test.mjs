import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { ApiError, createClient } from '../static/js/api.js';
import { createStore } from '../static/js/state.js';

function fakeFetch(responses) {
  const calls = [];
  const fetchImpl = async (url, init) => {
    calls.push({ url, ...init, body: init.body === undefined ? undefined : JSON.parse(init.body) });
    const r = responses.shift();
    return {
      ok: r.status >= 200 && r.status < 300,
      status: r.status,
      statusText: r.statusText ?? '',
      json: async () => {
        if (r.body === undefined) {
          throw new SyntaxError('no body');
        }
        return r.body;
      },
    };
  };
  return { fetchImpl, calls };
}

describe('api client', () => {
  it('maps every endpoint to method, path and body', async () => {
    const { fetchImpl, calls } = fakeFetch(Array.from({ length: 9 }, () => ({ status: 200, body: {} })));
    const c = createClient(fetchImpl, '/base');
    await c.listWheels();
    await c.createWheel({ name: 'a' });
    await c.updateWheel({ id: 3, name: 'b' });
    await c.deleteWheel(3);
    await c.listSessions(3);
    await c.createSession(3, [{ spoke_index: 1 }]);
    await c.getSession(4);
    await c.solveSession(4);
    await c.solve({ name: 'w' }, []);
    assert.deepEqual(calls.map((x) => `${x.method} ${x.url}`), [
      'GET /base/api/wheels', 'POST /base/api/wheels', 'PUT /base/api/wheels/3', 'DELETE /base/api/wheels/3',
      'GET /base/api/wheels/3/sessions', 'POST /base/api/wheels/3/sessions', 'GET /base/api/sessions/4',
      'POST /base/api/sessions/4/solve', 'POST /base/api/solve',
    ]);
    assert.deepEqual(calls[5].body, { measurements: [{ spoke_index: 1 }] });
    assert.deepEqual(calls[8].body, { wheel: { name: 'w' }, measurements: [] });
    assert.equal(calls[1].headers['Content-Type'], 'application/json');
    assert.equal(calls[0].headers['Content-Type'], undefined);
  });

  it('returns null for 204', async () => {
    const { fetchImpl } = fakeFetch([{ status: 204 }]);
    assert.equal(await createClient(fetchImpl).deleteWheel(1), null);
  });

  it('throws ApiError with server message and field', async () => {
    const { fetchImpl } = fakeFetch([{ status: 400, body: { error: 'must not be empty', field: 'name' } }]);
    await assert.rejects(createClient(fetchImpl).createWheel({}), (err) => {
      assert.ok(err instanceof ApiError);
      assert.equal(err.status, 400);
      assert.equal(err.field, 'name');
      assert.equal(err.message, 'name: must not be empty');
      return true;
    });
  });

  it('falls back to status text when the error body is not JSON', async () => {
    const { fetchImpl } = fakeFetch([{ status: 502, statusText: 'Bad Gateway' }, { status: 500 }]);
    const c = createClient(fetchImpl);
    await assert.rejects(c.listWheels(), { message: 'Bad Gateway', status: 502 });
    await assert.rejects(c.listWheels(), { message: 'HTTP 500' });
  });

  it('uses the global fetch by default', async () => {
    const original = globalThis.fetch;
    globalThis.fetch = fakeFetch([{ status: 200, body: [] }]).fetchImpl;
    try {
      assert.deepEqual(await createClient().listWheels(), []);
    } finally {
      globalThis.fetch = original;
    }
  });
});

describe('store', () => {
  it('merges patches immutably and notifies with previous state', () => {
    const st = createStore({ a: 1, b: 2 });
    const seen = [];
    const off = st.subscribe((next, prev) => seen.push([prev.a, next.a]));
    const before = st.get();
    st.set({ a: 5 });
    st.set((s) => ({ a: s.a + 1 }));
    assert.deepEqual(st.get(), { a: 6, b: 2 });
    assert.equal(before.a, 1);
    assert.ok(Object.isFrozen(st.get()));
    assert.deepEqual(seen, [[1, 5], [5, 6]]);
    off();
    st.set({ a: 0 });
    assert.equal(seen.length, 2);
  });
});
