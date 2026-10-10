import { computed, ref } from "vue";
import { defineStore } from "pinia";

import type { TenantInfo } from "@/api/tenant";

const PENDING_WORKSPACE_SETUP_KEY = "weknora_pending_workspace_setup";

export interface PendingWorkspaceSetup {
  tenantId: number;
  tenantName: string;
  tenantDescription?: string;
  idempotencyKey: string;
  createdRootOrg?: {
    id: string;
    name: string;
  };
}

function createIdempotencyKey(): string {
  if (
    typeof crypto !== "undefined" &&
    typeof crypto.randomUUID === "function"
  ) {
    return crypto.randomUUID();
  }
  return `ws-setup-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
}

function normalizePendingWorkspaceSetup(
  raw: unknown,
): PendingWorkspaceSetup | null {
  if (!raw || typeof raw !== "object") return null;
  const candidate = raw as Record<string, unknown>;
  const tenantId = Number(candidate.tenantId);
  const tenantName =
    typeof candidate.tenantName === "string" ? candidate.tenantName.trim() : "";
  if (!Number.isFinite(tenantId) || tenantId <= 0 || !tenantName) return null;
  const root = candidate.createdRootOrg;
  const normalizedRoot =
    root &&
    typeof root === "object" &&
    typeof (root as Record<string, unknown>).id === "string" &&
    typeof (root as Record<string, unknown>).name === "string"
      ? {
          id: ((root as Record<string, unknown>).id as string).trim(),
          name: ((root as Record<string, unknown>).name as string).trim(),
        }
      : undefined;
  return {
    tenantId,
    tenantName,
    tenantDescription:
      typeof candidate.tenantDescription === "string"
        ? candidate.tenantDescription
        : undefined,
    idempotencyKey:
      typeof candidate.idempotencyKey === "string" &&
      candidate.idempotencyKey.trim()
        ? candidate.idempotencyKey.trim()
        : createIdempotencyKey(),
    createdRootOrg:
      normalizedRoot?.id && normalizedRoot?.name ? normalizedRoot : undefined,
  };
}

export const useWorkspaceSetupStore = defineStore("workspaceSetup", () => {
  const pending = ref<PendingWorkspaceSetup | null>(null);

  const persist = () => {
    try {
      if (!pending.value) {
        sessionStorage.removeItem(PENDING_WORKSPACE_SETUP_KEY);
        return;
      }
      sessionStorage.setItem(
        PENDING_WORKSPACE_SETUP_KEY,
        JSON.stringify(pending.value),
      );
    } catch {
      // sessionStorage 不可用时退化为仅内存态；向导仍可在当前页继续工作。
    }
  };

  const start = (tenant: Pick<TenantInfo, "id" | "name" | "description">) => {
    pending.value = {
      tenantId: Number(tenant.id),
      tenantName: String(tenant.name || "").trim(),
      tenantDescription: tenant.description || undefined,
      idempotencyKey: createIdempotencyKey(),
    };
    persist();
  };

  const setCreatedRootOrg = (rootOrg: { id: string; name: string } | null) => {
    if (!pending.value) return;
    pending.value.createdRootOrg = rootOrg
      ? {
          id: String(rootOrg.id || "").trim(),
          name: String(rootOrg.name || "").trim(),
        }
      : undefined;
    persist();
  };

  const rotateIdempotencyKey = () => {
    if (!pending.value) return;
    pending.value.idempotencyKey = createIdempotencyKey();
    persist();
  };

  const clear = () => {
    pending.value = null;
    persist();
  };

  const hydratePending = () => {
    if (pending.value) return;
    try {
      const raw = sessionStorage.getItem(PENDING_WORKSPACE_SETUP_KEY);
      if (!raw) return;
      pending.value = normalizePendingWorkspaceSetup(JSON.parse(raw));
      if (!pending.value) {
        sessionStorage.removeItem(PENDING_WORKSPACE_SETUP_KEY);
      }
    } catch {
      try {
        sessionStorage.removeItem(PENDING_WORKSPACE_SETUP_KEY);
      } catch {
        // ignore
      }
    }
  };

  return {
    pending,
    active: computed(() => pending.value !== null),
    start,
    setCreatedRootOrg,
    rotateIdempotencyKey,
    clear,
    hydratePending,
  };
});
