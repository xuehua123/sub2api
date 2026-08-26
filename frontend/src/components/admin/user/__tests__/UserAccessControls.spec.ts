import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const currentDir = dirname(fileURLToPath(import.meta.url))
const editSource = readFileSync(resolve(currentDir, '../UserEditModal.vue'), 'utf8')
const groupsSource = readFileSync(resolve(currentDir, '../UserAllowedGroupsModal.vue'), 'utf8')
const keysSource = readFileSync(resolve(currentDir, '../../../../views/user/KeysView.vue'), 'utf8')
const plazaSource = readFileSync(resolve(currentDir, '../../../modelPlaza/PlazaGroupSection.vue'), 'utf8')

describe('admin user access controls', () => {
  it('edits and submits both user-level switches', () => {
    expect(editSource).toContain('data-test="restrict-to-allowed-groups-toggle"')
    expect(editSource).toContain('data-test="payment-disabled-toggle"')
    expect(editSource).toContain('u.restrict_to_allowed_groups ?? false')
    expect(editSource).toContain('u.payment_disabled ?? false')
    expect(editSource).toContain('restrict_to_allowed_groups: form.restrictToAllowedGroups')
    expect(editSource).toContain('payment_disabled: form.paymentDisabled')
  })

  it('shows public groups as unavailable in exclusive-only mode', () => {
    expect(groupsSource).toContain('user.restrict_to_allowed_groups')
    expect(groupsSource).toContain("t('admin.users.publicGroupsRestricted')")
    expect(groupsSource).toContain("!(props.user?.restrict_to_allowed_groups ?? false)")
  })

  it('keeps unavailable user-facing groups visible but disables their use', () => {
    expect(keysSource).toContain(':disabled="option.disabled"')
    expect(keysSource).toContain("t('keys.groupUnavailableExclusiveOnly')")
    expect(keysSource).toContain('selectedGroupOption.value?.disabled')
    expect(plazaSource).toContain('v-if="group.unavailable"')
    expect(plazaSource).toContain("t('modelPlaza.badges.viewOnly')")
  })
})
