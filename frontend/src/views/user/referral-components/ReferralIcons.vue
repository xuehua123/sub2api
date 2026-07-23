<script setup lang="ts">
/**
 * Minimal monochrome SVGs for referral marketing UI.
 * Keep strokes consistent; color via currentColor / class.
 */
defineProps<{
  name: 'gift' | 'link' | 'users' | 'wallet' | 'spark' | 'arrow' | 'convert' | 'check' | 'percent' | 'share'
  size?: number
}>()
</script>

<template>
  <svg
    :width="size || 20"
    :height="size || 20"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="1.75"
    stroke-linecap="round"
    stroke-linejoin="round"
    class="shrink-0"
    aria-hidden="true"
  >
    <!-- gift -->
    <template v-if="name === 'gift'">
      <rect x="3" y="8" width="18" height="13" rx="2" />
      <path d="M12 8v13M3 12h18" />
      <path d="M12 8c-1.5-3-4.5-3.5-5.5-2S5.5 9 7 9h5" />
      <path d="M12 8c1.5-3 4.5-3.5 5.5-2S18.5 9 17 9h-5" />
    </template>

    <!-- link -->
    <template v-else-if="name === 'link'">
      <path d="M10 13a5 5 0 007.54.54l3-3a5 5 0 00-7.07-7.07l-1.72 1.71" />
      <path d="M14 11a5 5 0 00-7.54-.54l-3 3a5 5 0 007.07 7.07l1.71-1.71" />
    </template>

    <!-- users -->
    <template v-else-if="name === 'users'">
      <path d="M16 21v-2a4 4 0 00-4-4H6a4 4 0 00-4 4v2" />
      <circle cx="9" cy="7" r="4" />
      <path d="M22 21v-2a4 4 0 00-3-3.87M16 3.13a4 4 0 010 7.75" />
    </template>

    <!-- wallet -->
    <template v-else-if="name === 'wallet'">
      <path d="M20 7H4a2 2 0 00-2 2v10a2 2 0 002 2h16a2 2 0 002-2V9a2 2 0 00-2-2z" />
      <path d="M16 3H6a2 2 0 00-2 2v2" />
      <circle cx="17" cy="14" r="1.25" fill="currentColor" stroke="none" />
    </template>

    <!-- spark -->
    <template v-else-if="name === 'spark'">
      <path d="M12 2l1.8 5.5L19.5 9.5 13.8 12 12 17.5 10.2 12 4.5 9.5l5.7-2L12 2z" />
      <path d="M19 14l.7 2.1L22 17l-2.3.9L19 20l-.7-2.1L16 17l2.3-.9L19 14z" />
    </template>

    <!-- arrow -->
    <template v-else-if="name === 'arrow'">
      <path d="M5 12h14M13 6l6 6-6 6" />
    </template>

    <!-- convert / swap -->
    <template v-else-if="name === 'convert'">
      <path d="M16 3l4 4-4 4" />
      <path d="M20 7H8a4 4 0 000 8h2" />
      <path d="M8 21l-4-4 4-4" />
      <path d="M4 17h12a4 4 0 000-8h-2" />
    </template>

    <!-- check -->
    <template v-else-if="name === 'check'">
      <circle cx="12" cy="12" r="9" />
      <path d="M8.5 12.5l2.5 2.5 4.5-5" />
    </template>

    <!-- percent -->
    <template v-else-if="name === 'percent'">
      <circle cx="8" cy="8" r="2.5" />
      <circle cx="16" cy="16" r="2.5" />
      <path d="M18 6L6 18" />
    </template>

    <!-- share -->
    <template v-else-if="name === 'share'">
      <circle cx="18" cy="5" r="2.5" />
      <circle cx="6" cy="12" r="2.5" />
      <circle cx="18" cy="19" r="2.5" />
      <path d="M8.3 10.8l7.4-4.6M8.3 13.2l7.4 4.6" />
    </template>
  </svg>
</template>
