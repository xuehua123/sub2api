import {describe,it,expect} from 'vitest'
import zh from '@/i18n/locales/zh/misc'
import en from '@/i18n/locales/en/misc'
describe('referral copy',()=>{
 it('does not render internal translation placeholders',()=>{for(const locale of [zh,en]){for(const [key,value] of Object.entries(locale.referral)){if(typeof value==='string')expect(value,key).not.toBe(key)}}})
 it('retains the account cooldown interpolation',()=>{expect(zh.referral.accountNextEditableAt).toContain('{time}');expect(en.referral.accountNextEditableAt).toContain('{time}')})
})
