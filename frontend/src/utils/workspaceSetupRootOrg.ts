export interface WorkspaceSetupRootOrgNode {
  id: string;
  name: string;
  parent_id: string | null;
  level: number;
  children?: WorkspaceSetupRootOrgNode[];
}

export interface ReusableRootOrg {
  id: string;
  name: string;
}

function flattenOrgTree(
  nodes: readonly WorkspaceSetupRootOrgNode[],
): WorkspaceSetupRootOrgNode[] {
  const out: WorkspaceSetupRootOrgNode[] = [];
  const stack = [...nodes].reverse();
  while (stack.length > 0) {
    const node = stack.pop();
    if (!node) continue;
    out.push(node);
    const children = Array.isArray(node.children) ? node.children : [];
    for (let i = children.length - 1; i >= 0; i -= 1) {
      stack.push(children[i]);
    }
  }
  return out;
}

function isRootOrgNode(node: WorkspaceSetupRootOrgNode): boolean {
  return !node.parent_id || node.parent_id.trim() === "" || node.level <= 1;
}

export function selectReusableRootOrg(
  nodes: readonly WorkspaceSetupRootOrgNode[],
  preferredNames: readonly string[] = [],
): ReusableRootOrg | null {
  const roots = flattenOrgTree(nodes).filter(isRootOrgNode);
  if (roots.length === 0) return null;

  const normalizedPreferredNames = preferredNames
    .map((name) => name.trim())
    .filter((name) => name.length > 0);

  for (const preferredName of normalizedPreferredNames) {
    const match = roots.find((root) => root.name.trim() === preferredName);
    if (match) {
      return { id: match.id, name: match.name };
    }
  }

  if (roots.length === 1) {
    return { id: roots[0].id, name: roots[0].name };
  }

  return null;
}
