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
    expect(source).toContain("case 'grok':")
    expect(source).toContain("return 'xai-...'")
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

  it('exposes custom upstream URL and header override for the OAuth create flow', () => {
    expect(source).toContain('data-testid="grok-custom-base-url-toggle"')
    expect(source).toContain('data-testid="grok-custom-base-url-input"')
    expect(source).toContain("form.platform === 'grok' && isOAuthFlow")
  })

  it('validates and applies upstream config on Grok OAuth create paths', () => {
    // 授权码兑换 / RT 批量 / SSO 批量（密码授权已隐藏）
    expect(source.match(/validateGrokOAuthUpstreamConfig\(\)/g)?.length).toBeGreaterThanOrEqual(3)
    expect(source.match(/applyGrokOAuthUpstreamConfig\(credentials\)/g)?.length).toBeGreaterThanOrEqual(3)
  })

  it('hides Grok password authorize option in the create flow', () => {
    expect(source).toContain(':show-email-password-option="false"')
  })

  it('persists the default-off Codex fingerprint mode for OAuth accounts', () => {
    expect(source).toContain("v-if=\"form.platform === 'openai' && form.type === 'oauth'\"")
    expect(source).toContain("const codexFingerprintMode = ref<CodexFingerprintMode>('off')")
    expect(source).toMatch(
      /if \(form\.type === 'oauth'\) \{\s+extra\.codex_fingerprint_mode = codexFingerprintMode\.value\s+\} else \{\s+delete extra\.codex_fingerprint_mode/
    )
  })

  it('does not overwrite an existing account fingerprint mode during an untouched Codex import', () => {
    expect(source).toContain('const codexFingerprintModeTouched = ref(false)')
    expect(source).toContain('@update:model-value="markCodexFingerprintModeTouched"')
    expect(source).toContain('const markCodexFingerprintModeTouched = () => {')
    expect(source).toMatch(
      /if \(!codexFingerprintModeTouched\.value\) \{\s+delete extra\.codex_fingerprint_mode/
    )
  })
})
