import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const source = readFileSync(
  resolve(process.cwd(), 'src/components/account/CreateAccountModal.vue'),
  'utf8'
)

describe('CreateAccountModal Grok account types', () => {
  it('offers API-key setup alongside OAuth with the official xAI default', () => {
    expect(source).toContain('data-testid="grok-account-type-api-key"')
    expect(source).toContain("@click=\"accountCategory = 'apikey'\"")
    expect(source).toMatch(
      /newPlatform === 'gemini'[\s\S]*?'https:\/\/generativelanguage\.googleapis\.com'[\s\S]*?newPlatform === 'grok'[\s\S]*?'https:\/\/api\.x\.ai\/v1'[\s\S]*?'https:\/\/api\.anthropic\.com'/
    )
    expect(source).toContain("form.platform === 'grok'")
    expect(source).toContain("? 'xai-...'")
  })

  it('keeps the complete Grok OAuth flow wired into the account modal', () => {
    expect(source).toContain("import { useGrokOAuth } from '@/composables/useGrokOAuth'")
    expect(source).toContain('const grokOAuth = useGrokOAuth()')
    expect(source).toContain("await grokOAuth.generateAuthUrl(form.proxy_id)")
    expect(source).toContain('handleGrokValidateRT(rt)')
    expect(source).toContain("case 'grok':")
    expect(source).toContain('return handleGrokExchange(authCode)')
    expect(source.match(/grokOAuth\.resetState\(\)/g)).toHaveLength(3)
  })

  it('keeps OpenAI Codex PAT import wired into the account modal', () => {
    expect(source).toContain(`:show-codex-pat-option="form.platform === 'openai'"`)
    expect(source).toContain('@import-codex-pat="handleOpenAIImportCodexPAT"')
    expect(source).toContain('const handleOpenAIImportCodexPAT = async (accessToken: string) => {')
    expect(source).toContain('await adminAPI.accounts.createOpenAICodexPAT({')
  })
})
