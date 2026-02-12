import { get, post, put, del } from '@/utils/request'

// ==================== Types ====================

export interface OrgTreeNode {
  id: string
  name: string
  description: string
  parent_id: string | null
  path: string
  level: number
  sort_order: number
  member_count: number
  my_is_admin: boolean
  children?: OrgTreeNode[]
  created_at: string
  updated_at: string
}

export interface CreateOrgTreeNodeRequest {
  name: string
  description?: string
  parent_id?: string | null
  sort_order?: number
}

export interface UpdateOrgTreeNodeRequest {
  name?: string
  description?: string
  sort_order?: number
}

export interface MoveOrgNodeRequest {
  new_parent_id?: string | null
  sort_order?: number
}

export interface AssignUserToOrgRequest {
  user_id: string
  role: 'admin' | 'editor' | 'viewer'
}

export interface SetOrgAdminRequest {
  user_id: string
  is_admin: boolean
}

export interface CreateUserInOrgRequest {
  username: string
  email?: string
  phone?: string
  password: string
  role: 'admin' | 'editor' | 'viewer'
}

export interface UpdateUserInOrgRequest {
  username: string
  email?: string
  phone?: string
  role?: 'admin' | 'editor' | 'viewer'
}

export interface CreateUserInOrgResponse {
  success: boolean
  message?: string
  user?: {
    id: string
    username: string
    email: string
    phone: string
  }
}

export interface OrgMember {
  user_id: string
  username: string
  email: string
  phone?: string
  role: string
  is_admin: boolean
  is_super_admin?: boolean
  joined_at: string
}

export interface SearchUserResult {
  id: string
  username: string
  email: string
  is_super_admin: boolean
}

export interface ApiResponse<T> {
  success: boolean
  data?: T
  message?: string
}

// ==================== API Functions ====================

/** Get the full organization tree (super admin only) */
export async function getOrgTree(): Promise<ApiResponse<OrgTreeNode[]>> {
  try {
    return await get('/api/v1/org-tree') as unknown as ApiResponse<OrgTreeNode[]>
  } catch (error: any) {
    return { success: false, message: error.message || 'Failed to get org tree' }
  }
}

/** Create a new org-tree node (super admin only) */
export async function createOrgTreeNode(data: CreateOrgTreeNodeRequest): Promise<ApiResponse<OrgTreeNode>> {
  try {
    return await post('/api/v1/org-tree', data) as unknown as ApiResponse<OrgTreeNode>
  } catch (error: any) {
    return { success: false, message: error.message || 'Failed to create org tree node' }
  }
}

/** Get a single org-tree node (super admin only) */
export async function getOrgTreeNode(id: string): Promise<ApiResponse<OrgTreeNode>> {
  try {
    return await get(`/api/v1/org-tree/${id}`) as unknown as ApiResponse<OrgTreeNode>
  } catch (error: any) {
    return { success: false, message: error.message || 'Failed to get org tree node' }
  }
}

/** Update an org-tree node (super admin only) */
export async function updateOrgTreeNode(id: string, data: UpdateOrgTreeNodeRequest): Promise<ApiResponse<OrgTreeNode>> {
  try {
    return await put(`/api/v1/org-tree/${id}`, data) as unknown as ApiResponse<OrgTreeNode>
  } catch (error: any) {
    return { success: false, message: error.message || 'Failed to update org tree node' }
  }
}

/** Delete an org-tree node (super admin only) */
export async function deleteOrgTreeNode(id: string): Promise<ApiResponse<null>> {
  try {
    return await del(`/api/v1/org-tree/${id}`) as unknown as ApiResponse<null>
  } catch (error: any) {
    return { success: false, message: error.message || 'Failed to delete org tree node' }
  }
}

/** Move an org-tree node (super admin only) */
export async function moveOrgTreeNode(id: string, data: MoveOrgNodeRequest): Promise<ApiResponse<null>> {
  try {
    return await post(`/api/v1/org-tree/${id}/move`, data) as unknown as ApiResponse<null>
  } catch (error: any) {
    return { success: false, message: error.message || 'Failed to move org tree node' }
  }
}

/** Assign a user to an org-tree node (super admin only) */
export async function assignUserToOrg(orgId: string, data: AssignUserToOrgRequest): Promise<ApiResponse<null>> {
  try {
    return await post(`/api/v1/org-tree/${orgId}/members`, data) as unknown as ApiResponse<null>
  } catch (error: any) {
    return { success: false, message: error.message || 'Failed to assign user to org' }
  }
}

/** Remove a user from an org-tree node (super admin only) */
export async function removeUserFromOrg(orgId: string, userId: string): Promise<ApiResponse<null>> {
  try {
    return await del(`/api/v1/org-tree/${orgId}/members/${userId}`) as unknown as ApiResponse<null>
  } catch (error: any) {
    return { success: false, message: error.message || 'Failed to remove user from org' }
  }
}

/** Set or unset org admin (super admin only) */
export async function setOrgAdmin(orgId: string, data: SetOrgAdminRequest): Promise<ApiResponse<null>> {
  try {
    return await put(`/api/v1/org-tree/${orgId}/admin`, data) as unknown as ApiResponse<null>
  } catch (error: any) {
    return { success: false, message: error.message || 'Failed to set org admin' }
  }
}

/** Get current user's org-tree organizations (all authenticated users) */
export async function getMyOrgTreeOrganizations(): Promise<ApiResponse<OrgTreeNode[]>> {
  try {
    return await get('/api/v1/my-organizations') as unknown as ApiResponse<OrgTreeNode[]>
  } catch (error: any) {
    return { success: false, message: error.message || 'Failed to get my organizations' }
  }
}

/** Get members of a specific org node (super admin only) */
export async function getOrgMembers(orgId: string): Promise<ApiResponse<OrgMember[]>> {
  try {
    return await get(`/api/v1/org-tree/${orgId}/members`) as unknown as ApiResponse<OrgMember[]>
  } catch (error: any) {
    return { success: false, message: error.message || 'Failed to get org members' }
  }
}

/** Search users for assignment (super admin only) */
export async function searchUsersForAssign(query: string, limit: number = 20): Promise<ApiResponse<SearchUserResult[]>> {
  try {
    return await get(`/api/v1/org-tree/search-users?q=${encodeURIComponent(query)}&limit=${limit}`) as unknown as ApiResponse<SearchUserResult[]>
  } catch (error: any) {
    return { success: false, message: error.message || 'Failed to search users' }
  }
}

/** Set or unset a user as super admin (super admin only) */
export async function setSuperAdmin(userId: string, isSuperAdmin: boolean): Promise<ApiResponse<null>> {
  try {
    return await put(`/api/v1/org-tree/super-admin`, { user_id: userId, is_super_admin: isSuperAdmin }) as unknown as ApiResponse<null>
  } catch (error: any) {
    return { success: false, message: error.message || 'Failed to set super admin' }
  }
}

/** Create a new user and assign to an organization (super admin only) */
export async function createUserInOrg(orgId: string, data: CreateUserInOrgRequest): Promise<CreateUserInOrgResponse> {
  try {
    return await post(`/api/v1/org-tree/${orgId}/create-user`, data) as unknown as CreateUserInOrgResponse
  } catch (error: any) {
    return { success: false, message: error.message || 'Failed to create user' }
  }
}

/** Update user information in an organization (super admin only) */
export async function updateUserInOrg(orgId: string, userId: string, data: UpdateUserInOrgRequest): Promise<CreateUserInOrgResponse> {
  try {
    return await put(`/api/v1/org-tree/${orgId}/users/${userId}`, data) as unknown as CreateUserInOrgResponse
  } catch (error: any) {
    return { success: false, message: error.message || 'Failed to update user' }
  }
}
