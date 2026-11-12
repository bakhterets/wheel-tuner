// Thin client for the JSON HTTP API.

export class ApiError extends Error {
  constructor(status, message, field = '') {
    super(field ? `${field}: ${message}` : message);
    this.name = 'ApiError';
    this.status = status;
    this.field = field;
  }
}

export function createClient(fetchImpl = (...args) => globalThis.fetch(...args), base = '') {
  async function request(method, path, body) {
    const init = { method, headers: { Accept: 'application/json' } };
    if (body !== undefined) {
      init.headers['Content-Type'] = 'application/json';
      init.body = JSON.stringify(body);
    }
    const res = await fetchImpl(base + path, init);
    if (res.status === 204) {
      return null;
    }
    let data = null;
    try {
      data = await res.json();
    } catch {
      data = null;
    }
    if (!res.ok) {
      throw new ApiError(res.status, data?.error || res.statusText || `HTTP ${res.status}`, data?.field);
    }
    return data;
  }

  return {
    listWheels: () => request('GET', '/api/wheels'),
    createWheel: (w) => request('POST', '/api/wheels', w),
    updateWheel: (w) => request('PUT', `/api/wheels/${w.id}`, w),
    deleteWheel: (id) => request('DELETE', `/api/wheels/${id}`),
    listSessions: (wheelId) => request('GET', `/api/wheels/${wheelId}/sessions`),
    createSession: (wheelId, measurements) => request('POST', `/api/wheels/${wheelId}/sessions`, { measurements }),
    getSession: (id) => request('GET', `/api/sessions/${id}`),
    solveSession: (id) => request('POST', `/api/sessions/${id}/solve`),
    solve: (wheel, measurements) => request('POST', '/api/solve', { wheel, measurements }),
  };
}
