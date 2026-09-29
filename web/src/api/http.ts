// Unified fetch wrapper: JWT token, {code,message,data} unwrapping, 401 redirect.
// All web API calls go through this file (rule for AI: never hand-roll fetch elsewhere).

import type { ApiResponse } from '@/types';

const TOKEN_KEY = 'edgetoken';

export function getToken(): string {
  return localStorage.getItem(TOKEN_KEY) || '';
}
export function setToken(t: string): void {
  localStorage.setItem(TOKEN_KEY, t);
}
export function clearToken(): void {
  localStorage.removeItem(TOKEN_KEY);
}

export class ApiError extends Error {
  code: number;
  constructor(code: number, message: string) {
    super(message);
    this.code = code;
  }
}

export async function request<T>(path: string, method = 'GET', body?: unknown): Promise<T> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' };
  const tok = getToken();
  if (tok) headers.Authorization = `Bearer ${tok}`;

  let resp: Response;
  try {
    resp = await fetch(path, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch {
    throw new ApiError(0, 'cannot reach core server (is core running on :8080?)');
  }
  const text = await resp.text();
  let env: ApiResponse<unknown>;
  try {
    env = JSON.parse(text) as ApiResponse<unknown>;
  } catch {
    throw new ApiError(resp.status, `non-JSON response from ${path}: ${text.slice(0, 160)}`);
  }

  if (resp.status === 401) {
    clearToken();
    if (!location.pathname.startsWith('/login')) location.href = '/login';
    throw new ApiError(401, env.message || 'unauthorized');
  }
  if (env.code !== 0) {
    throw new ApiError(env.code, env.message || 'request failed');
  }
  return env.data as T;
}

export const get = <T>(path: string): Promise<T> => request<T>(path);
export const post = <T>(path: string, body?: unknown): Promise<T> => request<T>(path, 'POST', body);
export const put = <T>(path: string, body?: unknown): Promise<T> => request<T>(path, 'PUT', body);
export const del = <T>(path: string): Promise<T> => request<T>(path, 'DELETE');
