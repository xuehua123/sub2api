<template>
  <div class="tabs-container">
    <div class="border-b border-gray-200 dark:border-dark-700">
      <nav class="-mb-px flex space-x-8" aria-label="Tabs">
        <button
          v-for="tab in tabs"
          :key="tab.value"
          @click="emit('update:modelValue', tab.value)"
          :class="[
            modelValue === tab.value
              ? 'border-indigo-500 text-indigo-600 dark:border-indigo-400 dark:text-indigo-400'
              : 'border-transparent text-gray-500 hover:border-gray-300 hover:text-gray-700 dark:text-dark-300 dark:hover:border-dark-600 dark:hover:text-dark-100',
            'group inline-flex items-center border-b-2 py-4 px-1 text-sm font-medium transition-colors whitespace-nowrap'
          ]"
          :aria-current="modelValue === tab.value ? 'page' : undefined"
        >
          <Icon
            v-if="tab.icon"
            :name="tab.icon"
            :class="[
              modelValue === tab.value
                ? 'text-indigo-500 dark:text-indigo-400'
                : 'text-gray-400 group-hover:text-gray-500 dark:text-dark-400 dark:group-hover:text-dark-300',
              '-ml-0.5 mr-2 h-5 w-5'
            ]"
            aria-hidden="true"
          />
          <span>{{ tab.label }}</span>
        </button>
      </nav>
    </div>
  </div>
</template>

<script setup lang="ts">
import Icon from '@/components/icons/Icon.vue'

export interface TabItem {
  label: string
  value: string | number
  icon?: any // icon name
}

interface Props {
  modelValue: string | number
  tabs: TabItem[]
}

defineProps<Props>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: string | number): void
}>()
</script>
