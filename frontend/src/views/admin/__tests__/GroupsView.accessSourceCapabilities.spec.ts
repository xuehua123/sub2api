import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { describe, expect, it } from "vitest";

const currentDir = dirname(fileURLToPath(import.meta.url));
const groupsViewSource = readFileSync(
  resolve(currentDir, "../GroupsView.vue"),
  "utf8",
);
const typesSource = readFileSync(
  resolve(currentDir, "../../../types/index.ts"),
  "utf8",
);

describe("admin groups access source capabilities", () => {
  it("exposes the three capability fields in group create and update payload types", () => {
    expect(typesSource).toContain("balance_enabled?: boolean");
    expect(typesSource).toContain("subscription_enabled?: boolean");
    expect(typesSource).toContain("plan_auto_grant_enabled?: boolean");
  });

  it("renders source capability controls instead of a billing type selector", () => {
    expect(groupsViewSource).toContain("admin.groups.capabilities.title");
    expect(groupsViewSource).toContain("admin.groups.capabilities.balanceEnabled");
    expect(groupsViewSource).toContain("admin.groups.capabilities.subscriptionEnabled");
    expect(groupsViewSource).toContain("admin.groups.capabilities.planAutoGrantEnabled");
    expect(groupsViewSource).not.toContain('v-model="createForm.subscription_type"');
    expect(groupsViewSource).not.toContain('v-model="editForm.subscription_type"');
  });

  it("derives legacy subscription_type from subscription capability", () => {
    expect(groupsViewSource).toContain(
      "form.subscription_enabled ? \"subscription\" : \"standard\"",
    );
    expect(groupsViewSource).toContain("normalizeGroupCapabilities(createForm)");
    expect(groupsViewSource).toContain("normalizeGroupCapabilities(editForm)");
  });

  it("submits the capability fields through create and update payloads", () => {
    expect(groupsViewSource).toMatch(/const requestData = \{\s+\.\.\.createGroupForm,/);
    expect(groupsViewSource).toContain("await adminAPI.groups.create(requestData)");
    expect(groupsViewSource).toMatch(/const payload = \{\s+\.\.\.editForm,/);
    expect(groupsViewSource).toContain(
      "await adminAPI.groups.update(editingGroup.value.id, payload)",
    );
    expect(groupsViewSource).toContain("balance_enabled: true");
    expect(groupsViewSource).toContain("subscription_enabled: false");
    expect(groupsViewSource).toContain("plan_auto_grant_enabled: false");
  });

  it("initializes edit capabilities from new fields with legacy subscription_type fallback", () => {
    expect(groupsViewSource).toContain(
      "group.balance_enabled ?? group.subscription_type !== \"subscription\"",
    );
    expect(groupsViewSource).toContain(
      "group.subscription_enabled ?? group.subscription_type === \"subscription\"",
    );
    expect(groupsViewSource).toContain(
      "editForm.plan_auto_grant_enabled = group.plan_auto_grant_enabled ?? false",
    );
    expect(groupsViewSource).toContain("normalizeGroupCapabilities(editForm)");
  });

  it("prevents plan auto grant when entitlement access is unavailable or group is not public active", () => {
    expect(groupsViewSource).toContain("const canEnablePlanAutoGrant");
    expect(groupsViewSource).toContain("form.subscription_enabled");
    expect(groupsViewSource).toContain("!form.is_exclusive");
    expect(groupsViewSource).toContain("(form.status ?? \"active\") === \"active\"");
    expect(groupsViewSource).toContain(":disabled=\"!canEnablePlanAutoGrant(createForm)\"");
    expect(groupsViewSource).toContain(":disabled=\"!canEnablePlanAutoGrant(editForm)\"");
  });

  it("keeps public visibility selectable for subscription-enabled groups", () => {
    expect(groupsViewSource).toContain('data-tour="group-form-exclusive"');
    expect(groupsViewSource).toContain("createForm.is_exclusive = !createForm.is_exclusive");
    expect(groupsViewSource).toContain("editForm.is_exclusive = !editForm.is_exclusive");
    expect(groupsViewSource).not.toContain(
      "v-if=\"createForm.subscription_type !== 'subscription'\"",
    );
    expect(groupsViewSource).not.toContain(
      "v-if=\"editForm.subscription_type !== 'subscription'\"",
    );
    expect(groupsViewSource).not.toContain("createForm.is_exclusive = true");
  });
});
