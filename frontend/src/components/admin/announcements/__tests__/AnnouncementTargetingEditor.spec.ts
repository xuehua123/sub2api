import { describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { mount } from '@vue/test-utils'

import AnnouncementTargetingEditor from '../AnnouncementTargetingEditor.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const UserSelectorStub = defineComponent({
  name: 'AnnouncementUserTargetSelector',
  emits: ['update:modelValue'],
  template: '<button data-testid="select-user" @click="$emit(\'update:modelValue\', [1012])">select</button>',
})

describe('AnnouncementTargetingEditor', () => {
  it('keeps an empty specific-user audience distinct from all users', async () => {
    const wrapper = mount(AnnouncementTargetingEditor, {
      props: {
        modelValue: { any_of: [] },
        groups: [],
      },
      global: {
        stubs: {
          AnnouncementUserTargetSelector: UserSelectorStub,
          GroupSelector: true,
          Select: true,
          Icon: true,
        },
      },
    })

    await wrapper.get('input[value="users"]').trigger('change')
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toEqual({ user_ids: [], any_of: [] })
	await wrapper.setProps({ modelValue: { user_ids: [], any_of: [] } })

    await wrapper.get('[data-testid="select-user"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toEqual({ user_ids: [1012], any_of: [] })
  })

  it('resets an unfinished user audience when a new all-user form is opened', async () => {
    const wrapper = mount(AnnouncementTargetingEditor, {
      props: { modelValue: { any_of: [] }, groups: [] },
      global: {
        stubs: {
          AnnouncementUserTargetSelector: UserSelectorStub,
          GroupSelector: true,
          Select: true,
          Icon: true,
        },
      },
    })

    await wrapper.get('input[value="users"]').trigger('change')
    await wrapper.setProps({ modelValue: { user_ids: [], any_of: [] } })
    expect(wrapper.get('input[value="users"]').attributes('checked')).toBeDefined()

    await wrapper.setProps({ modelValue: { any_of: [] } })
    expect(wrapper.get('input[value="all"]').attributes('checked')).toBeDefined()
  })
})
