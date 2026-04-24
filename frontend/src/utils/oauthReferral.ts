const OAUTH_REFERRAL_CODE_KEY = 'oauth_referral_code'

export function persistOAuthReferralCode(value: unknown): void {
  if (typeof window === 'undefined') return
  const code = typeof value === 'string' ? value.trim() : ''
  if (code) {
    window.sessionStorage.setItem(OAUTH_REFERRAL_CODE_KEY, code)
    return
  }
  window.sessionStorage.removeItem(OAUTH_REFERRAL_CODE_KEY)
}

export function getOAuthReferralCode(): string {
  if (typeof window === 'undefined') return ''
  return window.sessionStorage.getItem(OAUTH_REFERRAL_CODE_KEY)?.trim() || ''
}

export function clearOAuthReferralCode(): void {
  if (typeof window === 'undefined') return
  window.sessionStorage.removeItem(OAUTH_REFERRAL_CODE_KEY)
}
