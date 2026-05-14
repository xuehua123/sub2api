import { mount } from '@vue/test-utils'
import { defineComponent, nextTick, ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import IssueEmojiPickerButton from '../IssueEmojiPickerButton.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

const TestHost = defineComponent({
  components: { IssueEmojiPickerButton },
  setup() {
    const value = ref('Hello world')
    return { value }
  },
  template: `
    <div>
      <textarea id="emoji-target" v-model="value" data-testid="target"></textarea>
      <IssueEmojiPickerButton v-model="value" target-id="emoji-target" />
    </div>
  `,
})

describe('IssueEmojiPickerButton', () => {
  beforeEach(() => {
    window.localStorage.clear()
  })

  it('opens a categorized picker and inserts emoji at the cursor position', async () => {
    const wrapper = mount(TestHost, { attachTo: document.body })
    const target = wrapper.get('[data-testid="target"]').element as HTMLTextAreaElement
    target.focus()
    target.setSelectionRange(5, 5)

    await wrapper.get('[data-testid="issue-emoji-trigger"]').trigger('click')
    expect(wrapper.find('[data-testid="issue-emoji-picker"]').exists()).toBe(true)

    await wrapper.findAll('[data-testid="issue-emoji-option"]')[0].trigger('click')
    await nextTick()

    expect(target.value).toBe('Hello😀 world')
    expect(window.localStorage.getItem('support-issue-recent-emojis')).toContain('😀')

    wrapper.unmount()
  })
})
