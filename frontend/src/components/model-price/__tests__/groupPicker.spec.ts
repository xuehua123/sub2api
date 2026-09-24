import {describe,it,expect} from 'vitest'
import {mount} from '@vue/test-utils'
import Picker from '../ModelPriceGroupPicker.vue'
import {response} from './fixtures'
describe('quick group picker',()=>{
 it('selects a group in one click and marks current choice',async()=>{const w=mount(Picker,{props:{groups:response().groups,selectedID:46,busy:false},global:{stubs:{Icon:true}}});expect(w.get('[data-testid="quick-group-46"]').attributes('aria-pressed')).toBe('true');await w.get('[data-testid="quick-group-47"]').trigger('click');expect(w.emitted('select')![0]).toEqual([47]);w.unmount()})
 it('searches groups and clears an empty result',async()=>{const w=mount(Picker,{props:{groups:response().groups,busy:false},global:{stubs:{Icon:true}}});await w.get('input').setValue('Claude');expect(w.find('[data-testid="quick-group-46"]').exists()).toBe(false);await w.get('input').setValue('missing');expect(w.text()).toContain('没有匹配');await w.findAll('button').find(b=>b.text()==='清除筛选')!.trigger('click');expect(w.find('[data-testid="quick-group-46"]').exists()).toBe(true);w.unmount()})
 it('locks selection while saving',()=>{const w=mount(Picker,{props:{groups:response().groups,busy:true},global:{stubs:{Icon:true}}});expect(w.get('[data-testid="quick-group-46"]').attributes('disabled')).toBeDefined();w.unmount()})
})
