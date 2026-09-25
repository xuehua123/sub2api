import { it, expect, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import FX from '../UserBusinessFX.vue'
const { getBusinessFX, saveBusinessFX } = vi.hoisted(() => ({
  getBusinessFX: vi.fn(),
  saveBusinessFX: vi.fn(),
}))
vi.mock('@/api/admin/userBusiness', () => ({ getBusinessFX, saveBusinessFX }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
it('records explicit historical evidence and emits refresh only on success', async () => {
  getBusinessFX.mockResolvedValue([
    { date: '2026-09-24', usd_cny: null, source: null },
  ])
  saveBusinessFX.mockResolvedValue(undefined)
  const w = mount(FX, {
    props: { show: false, start: '2026-09-24', end: '2026-09-25' },
    global: {
      stubs: {
        BaseDialog: {
          props: ['show'],
          template: '<div v-if="show"><slot/></div>',
        },
      },
    },
  })
  await w.setProps({ show: true })
  await flushPromises()
  expect(w.text()).toContain('userBusiness.fxMissing')
  await w.get('input[type="number"]').setValue('6.5')
  await w.get('input[maxlength]').setValue('Settlement receipt #1')
  await w.get('form').trigger('submit')
  await flushPromises()
  expect(saveBusinessFX).toHaveBeenCalledWith({
    date: '2026-09-24',
    usd_cny: 6.5,
    source: 'Settlement receipt #1',
  })
  expect(w.emitted('saved')).toHaveLength(1)
  saveBusinessFX.mockRejectedValue(new Error('existing rate'))
  await w.get('form').trigger('submit')
  await flushPromises()
  expect(w.emitted('saved')).toHaveLength(1)
  expect(w.text()).toContain('userBusiness.fxSaveError')
  w.unmount()
})
