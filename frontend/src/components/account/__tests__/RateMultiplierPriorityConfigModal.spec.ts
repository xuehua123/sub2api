import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import RateMultiplierPriorityConfigModal from '../RateMultiplierPriorityConfigModal.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})

function mountModal() {
  return mount(RateMultiplierPriorityConfigModal, {
    props: {
      show: true,
      settings: {
        enabled: true,
        interval_minutes: 5,
        priority_step: 3
      }
    },
    global: {
      stubs: { BaseDialog: BaseDialogStub }
    }
  })
}

describe('RateMultiplierPriorityConfigModal', () => {
  it('saves the configured interval and priority step', async () => {
    const wrapper = mountModal()

    await wrapper.get('#rate-priority-interval').setValue(12)
    await wrapper.get('#rate-priority-step').setValue(20)
    await wrapper.get('form').trigger('submit')

    expect(wrapper.emitted('save')).toEqual([[{
      enabled: true,
      interval_minutes: 12,
      priority_step: 20
    }]])
  })

  it('does not submit an invalid step', async () => {
    const wrapper = mountModal()

    await wrapper.get('#rate-priority-step').setValue(0)
    await wrapper.get('form').trigger('submit')

    expect(wrapper.emitted('save')).toBeUndefined()
  })
})
