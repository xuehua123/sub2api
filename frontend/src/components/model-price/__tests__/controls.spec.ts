import { describe, it, expect, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import ModelPriceCalculator from '../ModelPriceCalculator.vue'
import ModelPriceEditor from '../ModelPriceEditor.vue'
import ModelPriceOperations from '../ModelPriceOperations.vue'
import { model, response } from './fixtures'
const stubs = {
  BaseDialog: {
    props: ['show'],
    template: '<div v-if="show"><slot/><slot name="footer"/></div>',
  },
  Icon: true,
  RouterLink: true,
}
describe('pricing controls', () => {
  it('uses the request count in live token estimates', async () => {
    const w = mount(ModelPriceCalculator, { props: { models: [model()] } })
    await w.get('[data-testid="calculator-input"]').setValue(1000000)
    await w.get('[data-testid="calculator-output"]').setValue(100000)
    await w.get('[data-testid="calculator-requests"]').setValue(3)
    expect(w.get('[data-testid="calculator-total"]').text()).toContain('17.64')
    w.unmount()
  })
  it('does not turn missing input rates into zero', async () => {
    const m = model()
    m.actual.input_cny_per_m = null
    const w = mount(ModelPriceCalculator, { props: { models: [m] } })
    expect(w.get('[data-testid="calculator-total"]').text()).toBe('暂不可估算')
    w.unmount()
  })
  it('preserves dirty conversion fields across background response refresh', async () => {
    const saveSettings = vi.fn().mockResolvedValue(true)
    const w = mount(ModelPriceOperations, {
      props: {
        response: response(),
        busy: false,
        lastAction: '',
        saveSettings,
      },
      global: { stubs },
    })
    await w.findAll('input')[0].setValue(8)
    await w.setProps({ response: response({ usd_cny_rate: 9 }) })
    expect((w.findAll('input')[0].element as HTMLInputElement).value).toBe('8')
    w.unmount()
  })
  it('requires clear confirmation and preserves its original target', async () => {
    const m = model({ custom_price: { input_usd_per_m: 1 } })
    const w = mount(ModelPriceEditor, {
      props: { model: m, groupID: 46, groupName: 'A', busy: false },
      global: { stubs },
    })
    await w
      .findAll('button')
      .find((b) => b.text() === '清除覆盖')!
      .trigger('click')
    expect(w.emitted('save')).toBeUndefined()
    await w
      .findAll('button')
      .find((b) => b.text() === '确认清除覆盖')!
      .trigger('click')
    expect(w.emitted('save')![0]).toEqual([
      { group_id: 46, model: m.name, clear: true },
    ])
    w.unmount()
  })
  it('does not emit a save until a valid preview has been accepted', async () => {
    const w = mount(ModelPriceEditor, {
      props: { model: model(), groupID: 46, groupName: 'A', busy: false },
      global: { stubs },
    })
    await w.get('[data-testid="price-editor-input_usd_per_m"]').setValue(-1)
    await w.get('#model-price-edit').trigger('submit')
    await flushPromises()
    expect(w.find('[data-testid="price-editor-review"]').exists()).toBe(false)
    expect(w.emitted('save')).toBeUndefined()
    w.unmount()
  })
})
