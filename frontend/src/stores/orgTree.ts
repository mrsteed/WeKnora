import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { OrgTreeNode, CreateOrgTreeNodeRequest, UpdateOrgTreeNodeRequest, MoveOrgNodeRequest, AssignUserToOrgRequest, SetOrgAdminRequest } from '@/api/org-tree'
import {
  getOrgTree,
  createOrgTreeNode,
  updateOrgTreeNode,
  deleteOrgTreeNode,
  moveOrgTreeNode,
  assignUserToOrg,
  removeUserFromOrg,
  setOrgAdmin
} from '@/api/org-tree'

export const useOrgTreeStore = defineStore('orgTree', () => {
  const tree = ref<OrgTreeNode[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function fetchTree() {
    loading.value = true
    error.value = null
    try {
      const response = await getOrgTree()
      if (response.success && response.data) {
        tree.value = response.data
      } else {
        error.value = response.message || 'Failed to fetch org tree'
      }
    } catch (e: any) {
      error.value = e.message || 'Failed to fetch org tree'
    } finally {
      loading.value = false
    }
  }

  async function createNode(data: CreateOrgTreeNodeRequest) {
    loading.value = true
    error.value = null
    try {
      const response = await createOrgTreeNode(data)
      if (response.success) {
        await fetchTree()
        return response.data
      } else {
        error.value = response.message || 'Failed to create node'
        return null
      }
    } catch (e: any) {
      error.value = e.message || 'Failed to create node'
      return null
    } finally {
      loading.value = false
    }
  }

  async function updateNode(id: string, data: UpdateOrgTreeNodeRequest) {
    loading.value = true
    error.value = null
    try {
      const response = await updateOrgTreeNode(id, data)
      if (response.success) {
        await fetchTree()
        return response.data
      } else {
        error.value = response.message || 'Failed to update node'
        return null
      }
    } catch (e: any) {
      error.value = e.message || 'Failed to update node'
      return null
    } finally {
      loading.value = false
    }
  }

  async function deleteNode(id: string) {
    loading.value = true
    error.value = null
    try {
      const response = await deleteOrgTreeNode(id)
      if (response.success) {
        await fetchTree()
        return true
      } else {
        error.value = response.message || 'Failed to delete node'
        return false
      }
    } catch (e: any) {
      error.value = e.message || 'Failed to delete node'
      return false
    } finally {
      loading.value = false
    }
  }

  async function moveNode(id: string, data: MoveOrgNodeRequest) {
    loading.value = true
    error.value = null
    try {
      const response = await moveOrgTreeNode(id, data)
      if (response.success) {
        await fetchTree()
        return true
      } else {
        error.value = response.message || 'Failed to move node'
        return false
      }
    } catch (e: any) {
      error.value = e.message || 'Failed to move node'
      return false
    } finally {
      loading.value = false
    }
  }

  async function assignUser(orgId: string, data: AssignUserToOrgRequest) {
    try {
      const response = await assignUserToOrg(orgId, data)
      if (response.success) {
        await fetchTree()
        return true
      }
      return false
    } catch {
      return false
    }
  }

  async function removeUser(orgId: string, userId: string) {
    try {
      const response = await removeUserFromOrg(orgId, userId)
      if (response.success) {
        await fetchTree()
        return true
      }
      return false
    } catch {
      return false
    }
  }

  async function setAdmin(orgId: string, data: SetOrgAdminRequest) {
    try {
      const response = await setOrgAdmin(orgId, data)
      if (response.success) {
        await fetchTree()
        return true
      }
      return false
    } catch {
      return false
    }
  }

  function clearState() {
    tree.value = []
    error.value = null
  }

  return {
    tree,
    loading,
    error,
    fetchTree,
    createNode,
    updateNode,
    deleteNode,
    moveNode,
    assignUser,
    removeUser,
    setAdmin,
    clearState
  }
})
