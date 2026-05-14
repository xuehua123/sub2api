import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import IssueMarkdownContent from '../IssueMarkdownContent.vue'

describe('IssueMarkdownContent', () => {
  it('renders common markdown formatting', () => {
    const wrapper = mount(IssueMarkdownContent, {
      props: {
        content: '**原因**\n\n- 请求超时\n- `429`\n\n[查看文档](/docs)',
      },
    })

    expect(wrapper.html()).toContain('<strong>原因</strong>')
    expect(wrapper.html()).toContain('<li>请求超时</li>')
    expect(wrapper.html()).toContain('<code>429</code>')
    expect(wrapper.get('a').attributes('href')).toBe('/docs')
  })

  it('escapes raw html and strips unsafe markdown output', () => {
    const wrapper = mount(IssueMarkdownContent, {
      props: {
        content: '<img src=x onerror=alert(1)>\n\n<script>alert(1)</script>\n\n![bad](javascript:alert(1))',
      },
    })

    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.find('script').exists()).toBe(false)
    expect(wrapper.html()).not.toContain('javascript:alert')
    expect(wrapper.text()).toContain('<img src=x onerror=alert(1)>')
  })
})
