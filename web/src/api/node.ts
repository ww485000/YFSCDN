// Edge node API — /api/v1/admin/nodes (CRUD) & /api/v1/user/nodes (read-only).
import { del, get, post, put } from './http';
import type { EdgeNode, PageData } from '@/types';

export function listNodesAdmin(q: { tenantId?: number; keyword?: string; page?: number; size?: number } = {}): Promise<
  PageData<EdgeNode>
> {
  const p = new URLSearchParams();
  if (q.tenantId) p.set('tenant_id', String(q.tenantId));
  if (q.keyword) p.set('keyword', q.keyword);
  p.set('page', String(q.page || 1));
  p.set('size', String(q.size || 20));
  return get(`/api/v1/admin/nodes?${p.toString()}`);
}

export function createNode(b: { name: string; tenant_id: number }): Promise<EdgeNode> {
  return post('/api/v1/admin/nodes', b);
}

export function updateNode(id: number, b: { name: string; tenant_id: number }): Promise<unknown> {
  return put(`/api/v1/admin/nodes/${id}`, b);
}

export function deleteNode(id: number): Promise<unknown> {
  return del(`/api/v1/admin/nodes/${id}`);
}

export function listNodesTenant(): Promise<PageData<EdgeNode>> {
  return get('/api/v1/user/nodes');
}
