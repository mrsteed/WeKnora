import assert from "node:assert/strict";
import test from "node:test";

import {
  selectReusableRootOrg,
  type WorkspaceSetupRootOrgNode,
} from "./workspaceSetupRootOrg";

test("selectReusableRootOrg reuses the only root node when the tree already exists", () => {
  const tree: WorkspaceSetupRootOrgNode[] = [
    {
      id: "root-1",
      name: "某支队",
      parent_id: null,
      level: 1,
      children: [
        {
          id: "child-1",
          name: "一中队",
          parent_id: "root-1",
          level: 2,
        },
      ],
    },
  ];

  assert.deepEqual(selectReusableRootOrg(tree, []), {
    id: "root-1",
    name: "某支队",
  });
});

test("selectReusableRootOrg prefers an exact root-name match when multiple roots exist", () => {
  const tree: WorkspaceSetupRootOrgNode[] = [
    { id: "root-1", name: "支队A", parent_id: null, level: 1 },
    { id: "root-2", name: "支队B", parent_id: null, level: 1 },
  ];

  assert.deepEqual(selectReusableRootOrg(tree, ["支队B"]), {
    id: "root-2",
    name: "支队B",
  });
});

test("selectReusableRootOrg stays conservative when multiple roots exist without a trusted match", () => {
  const tree: WorkspaceSetupRootOrgNode[] = [
    { id: "root-1", name: "支队A", parent_id: null, level: 1 },
    { id: "root-2", name: "支队B", parent_id: null, level: 1 },
  ];

  assert.equal(selectReusableRootOrg(tree, ["未知根组织"]), null);
});
