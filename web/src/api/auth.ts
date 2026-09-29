// Auth API — /api/v1/auth/*.
import { get, post } from './http';
import type { LoginResult, Me } from '@/types';

export function login(body: { username: string; password: string; scope: 'admin' | 'user' }): Promise<LoginResult> {
  return post<LoginResult>('/api/v1/auth/login', body);
}

export function me(): Promise<Me> {
  return get<Me>('/api/v1/auth/me');
}

export function changeTenantPassword(body: { old_password: string; new_password: string }): Promise<unknown> {
  return post('/api/v1/user/password', body);
}
