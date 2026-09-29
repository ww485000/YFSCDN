// Task queue API (admin only).
import { get } from './http';
import type { PageData, Task } from '@/types';

export function listTasks(
  q: { type?: string; status?: string; page?: number; size?: number } = {},
): Promise<PageData<Task>> {
  const p = new URLSearchParams();
  if (q.type) p.set('type', q.type);
  if (q.status) p.set('status', q.status);
  p.set('page', String(q.page || 1));
  p.set('size', String(q.size || 20));
  return get(`/api/v1/admin/tasks?${p.toString()}`);
}
