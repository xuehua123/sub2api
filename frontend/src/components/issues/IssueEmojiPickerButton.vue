<template>
  <div ref="root" class="relative inline-flex">
    <button
      class="inline-flex h-8 w-8 items-center justify-center rounded-full border border-gray-200 bg-white text-base shadow-sm transition hover:border-primary-300 hover:bg-primary-50 focus:outline-none focus:ring-2 focus:ring-primary-500/40 disabled:cursor-not-allowed disabled:opacity-50 dark:border-dark-600 dark:bg-dark-900 dark:hover:border-primary-700 dark:hover:bg-primary-950/40"
      type="button"
      :aria-label="t('issueCenter.emoji.button')"
      :aria-expanded="open"
      :disabled="disabled"
      data-testid="issue-emoji-trigger"
      @mousedown.prevent.stop
      @click.stop="toggle"
    >
      ☺
    </button>

    <Transition
      enter-active-class="transition duration-150 ease-out"
      enter-from-class="scale-95 opacity-0"
      enter-to-class="scale-100 opacity-100"
      leave-active-class="transition duration-100 ease-in"
      leave-from-class="scale-100 opacity-100"
      leave-to-class="scale-95 opacity-0"
    >
      <div
        v-if="open"
        class="absolute right-0 top-full z-40 mt-2 w-[min(22rem,calc(100vw-2rem))] origin-top-right overflow-hidden rounded-xl border border-gray-200 bg-white shadow-2xl ring-1 ring-black/5 dark:border-dark-600 dark:bg-dark-800 dark:ring-white/10"
        data-testid="issue-emoji-picker"
        @mousedown.prevent.stop
      >
        <div class="border-b border-gray-200 bg-gradient-to-r from-primary-50 via-white to-cyan-50 px-3 py-3 dark:border-dark-600 dark:from-primary-950/40 dark:via-dark-800 dark:to-cyan-950/30">
          <div class="flex items-center justify-between gap-3">
            <div>
              <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('issueCenter.emoji.title') }}</p>
              <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ t('issueCenter.emoji.hint') }}</p>
            </div>
            <button
              class="rounded-full px-2 py-1 text-xs font-medium text-gray-500 transition hover:bg-white hover:text-gray-900 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-white"
              type="button"
              :aria-label="t('issueCenter.emoji.close')"
              @click.stop="close"
            >
              Esc
            </button>
          </div>

          <div class="mt-3 flex gap-1 overflow-x-auto pb-0.5">
            <button
              v-for="group in visibleGroups"
              :key="group.id"
              class="shrink-0 rounded-full px-3 py-1.5 text-xs font-semibold transition"
              :class="activeGroupID === group.id ? 'bg-primary-600 text-white shadow-sm dark:bg-primary-500' : 'bg-white/80 text-gray-600 hover:bg-white hover:text-gray-900 dark:bg-dark-900/70 dark:text-gray-300 dark:hover:bg-dark-700 dark:hover:text-white'"
              type="button"
              @click.stop="activeGroupID = group.id"
            >
              {{ t(group.labelKey) }}
            </button>
          </div>
        </div>

        <div class="grid max-h-64 grid-cols-8 gap-1 overflow-y-auto p-3">
          <button
            v-for="emoji in activeEmojis"
            :key="`${activeGroupID}-${emoji}`"
            class="flex aspect-square items-center justify-center rounded-lg text-xl transition hover:bg-primary-50 hover:scale-110 focus:outline-none focus:ring-2 focus:ring-primary-500/40 dark:hover:bg-primary-950/40"
            type="button"
            data-testid="issue-emoji-option"
            @click.stop="selectEmoji(emoji)"
          >
            {{ emoji }}
          </button>
        </div>

        <div class="border-t border-gray-200 px-3 py-2 text-xs text-gray-500 dark:border-dark-600 dark:text-gray-400">
          {{ t('issueCenter.emoji.footer') }}
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

type EmojiGroup = {
  id: string
  labelKey: string
  emojis: string[]
}

const props = defineProps<{
  modelValue?: string
  targetId: string
  disabled?: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
  select: [emoji: string]
}>()

const { t } = useI18n()

const storageKey = 'support-issue-recent-emojis'
const maxRecent = 16
const root = ref<HTMLElement | null>(null)
const open = ref(false)
const recentEmojis = ref<string[]>([])
const activeGroupID = ref('mood')

const baseGroups: EmojiGroup[] = [
  {
    id: 'mood',
    labelKey: 'issueCenter.emoji.groups.mood',
    emojis: ['😀', '😄', '😅', '😂', '🙂', '😊', '😌', '😍', '🤩', '😎', '🥳', '😭', '😤', '😵‍💫', '🤔', '🙏'],
  },
  {
    id: 'reaction',
    labelKey: 'issueCenter.emoji.groups.reaction',
    emojis: ['👍', '👎', '👌', '👏', '🙌', '💪', '👀', '💬', '💯', '✅', '❌', '⚠️', '🚨', '💥', '✨', '🎉'],
  },
  {
    id: 'status',
    labelKey: 'issueCenter.emoji.groups.status',
    emojis: ['🟢', '🟡', '🔴', '🔵', '🟣', '⚪', '⬆️', '⬇️', '➡️', '🔁', '⏳', '⏱️', '⚡', '🔥', '🧊', '🧯'],
  },
  {
    id: 'work',
    labelKey: 'issueCenter.emoji.groups.work',
    emojis: ['💡', '📌', '📎', '📷', '🧾', '📊', '🔍', '🔧', '🧪', '🧩', '🔐', '🔑', '💳', '💰', '🚀', '🏁'],
  },
]

const visibleGroups = computed<EmojiGroup[]>(() => {
  if (!recentEmojis.value.length) return baseGroups
  return [
    {
      id: 'recent',
      labelKey: 'issueCenter.emoji.groups.recent',
      emojis: recentEmojis.value,
    },
    ...baseGroups,
  ]
})

const activeEmojis = computed(() => {
  return visibleGroups.value.find((group) => group.id === activeGroupID.value)?.emojis ?? baseGroups[0].emojis
})

watch(recentEmojis, () => {
  if (activeGroupID.value === 'recent' && !recentEmojis.value.length) {
    activeGroupID.value = 'mood'
  }
})

function toggle() {
  if (props.disabled) return
  open.value = !open.value
}

function close() {
  open.value = false
  focusTarget()
}

function selectEmoji(emoji: string) {
  const value = props.modelValue ?? ''
  const target = getTarget()
  const start = target?.selectionStart ?? value.length
  const end = target?.selectionEnd ?? start
  const nextValue = `${value.slice(0, start)}${emoji}${value.slice(end)}`
  emit('update:modelValue', nextValue)
  emit('select', emoji)
  rememberEmoji(emoji)

  nextTick(() => {
    const nextTarget = getTarget()
    if (!nextTarget) return
    const cursor = start + emoji.length
    nextTarget.focus()
    nextTarget.setSelectionRange(cursor, cursor)
  })
}

function getTarget(): HTMLInputElement | HTMLTextAreaElement | null {
  const target = document.getElementById(props.targetId)
  if (target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement) {
    return target
  }
  return null
}

function focusTarget() {
  nextTick(() => {
    getTarget()?.focus()
  })
}

function rememberEmoji(emoji: string) {
  recentEmojis.value = [emoji, ...recentEmojis.value.filter((item) => item !== emoji)].slice(0, maxRecent)
  try {
    window.localStorage.setItem(storageKey, JSON.stringify(recentEmojis.value))
  } catch {
    // Ignore storage failures; the picker still works without recents.
  }
}

function loadRecentEmojis() {
  try {
    const raw = window.localStorage.getItem(storageKey)
    if (!raw) return
    const parsed = JSON.parse(raw)
    if (Array.isArray(parsed)) {
      recentEmojis.value = parsed.filter((item): item is string => typeof item === 'string').slice(0, maxRecent)
    }
  } catch {
    recentEmojis.value = []
  }
}

function handleDocumentPointerDown(event: PointerEvent) {
  if (!open.value || !root.value) return
  if (event.target instanceof Node && root.value.contains(event.target)) return
  open.value = false
}

function handleDocumentKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    open.value = false
  }
}

onMounted(() => {
  loadRecentEmojis()
  document.addEventListener('pointerdown', handleDocumentPointerDown)
  document.addEventListener('keydown', handleDocumentKeydown)
})

onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', handleDocumentPointerDown)
  document.removeEventListener('keydown', handleDocumentKeydown)
})
</script>
