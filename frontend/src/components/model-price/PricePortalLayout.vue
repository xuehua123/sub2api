<template>
  <div class="ppx-price-portal" :data-theme="theme">
    <a class="ppx-price-skip" href="#price-main">跳到主要内容</a>
    <header class="ppx-price-header">
      <a href="/home" class="ppx-price-brand"
        ><img
          :src="logo"
          alt=""
          width="32"
          height="32"
          @error="fallbackLogo"
        /><strong>{{ name }}</strong
        ><span>{{ sectionLabel }}</span></a
      >
      <nav class="ppx-price-nav" aria-label="站点导航">
        <a href="/home">首页</a
        ><RouterLink
          to="/model-plaza"
          :aria-current="publicPage ? 'page' : undefined"
          >模型与价格</RouterLink
        ><a :href="docs" target="_blank" rel="noopener noreferrer"
          >开发文档<Icon name="externalLink" size="xs"
        /></a>
      </nav>
      <div class="ppx-price-actions">
        <button
          type="button"
          class="ppx-theme-toggle"
          :aria-label="theme === 'dark' ? '切换浅色主题' : '切换深色主题'"
          :title="theme === 'dark' ? '切换浅色主题' : '切换深色主题'"
          @click="toggle"
        >
          <Icon :name="theme === 'dark' ? 'sun' : 'moon'" size="md" /></button
        ><RouterLink
          :to="
            auth.isAuthenticated
              ? auth.isAdmin
                ? '/admin/dashboard'
                : '/dashboard'
              : '/login'
          "
          class="ppx-console-link"
          >{{ auth.isAuthenticated ? '控制台' : '登录'
          }}<Icon name="arrowRight" size="sm"
        /></RouterLink>
      </div>
    </header>
    <div class="ppx-price-body"><slot /></div>
    <footer class="ppx-price-footer">
      <span>{{ name }} / MODEL GATEWAY</span>
      <div>
        <a :href="docs" target="_blank" rel="noopener noreferrer">开发文档</a
        ><RouterLink to="/monitor">服务状态</RouterLink>
      </div>
    </footer>
  </div>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore, useAuthStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'
import './price-portal.css'
withDefaults(defineProps<{ publicPage?: boolean; sectionLabel?: string }>(), {sectionLabel:'模型价格'})
const app = useAppStore(),
  auth = useAuthStore()
const name = computed(() => app.siteName || '皮皮虾 AI')
const logo = computed(
  () =>
    sanitizeUrl(app.siteLogo || '', {
      allowRelative: true,
      allowDataUrl: true,
    }) || '/brand/ppx-logo.webp',
)
const docs = computed(
  () =>
    sanitizeUrl(app.docUrl || '', { allowRelative: true }) ||
    'https://doc.psydo.top/',
)
function initialTheme(): 'dark' | 'light' {
  try {
    const value = localStorage.getItem('ppx-theme')
    if (value === 'light' || value === 'dark') return value
  } catch {
    // Restricted storage must not block the public page.
  }
  return 'dark'
}
const theme = ref(initialTheme())
let previousDark = false,
  previousPortal = false
function apply() {
  document.documentElement.classList.toggle('dark', theme.value === 'dark')
}
function toggle() {
  theme.value = theme.value === 'dark' ? 'light' : 'dark'
  try {
    localStorage.setItem('ppx-theme', theme.value)
  } catch {
    // Keep the selected appearance for this visit when persistence is blocked.
  }
  apply()
}
function syncTheme(event: StorageEvent) {
  if (event.key === 'ppx-theme') {
    theme.value = initialTheme()
    apply()
  }
}
function fallbackLogo(event: Event) {
  const image = event.target as HTMLImageElement
  if (!image.src.endsWith('/brand/ppx-logo.webp'))
    image.src = '/brand/ppx-logo.webp'
}
onMounted(() => {
  previousDark = document.documentElement.classList.contains('dark')
  previousPortal = document.documentElement.classList.contains('ppx-price-page')
  document.documentElement.classList.add('ppx-price-page')
  apply()
  window.addEventListener('storage', syncTheme)
})
onBeforeUnmount(() => {
  window.removeEventListener('storage', syncTheme)
  document.documentElement.classList.toggle('dark', previousDark)
  document.documentElement.classList.toggle('ppx-price-page', previousPortal)
})
</script>
