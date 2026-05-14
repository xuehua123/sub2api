<template>
  <div class="issue-markdown-content break-words text-sm leading-6" v-html="renderedHtml"></div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { renderIssueMarkdown } from '@/utils/issueMarkdown'

const props = defineProps<{
  content?: string | null
}>()

const renderedHtml = computed(() => renderIssueMarkdown(props.content))
</script>

<style scoped>
.issue-markdown-content :deep(*) {
  @apply max-w-full;
}

.issue-markdown-content :deep(p) {
  @apply my-2;
}

.issue-markdown-content :deep(p:first-child) {
  @apply mt-0;
}

.issue-markdown-content :deep(p:last-child) {
  @apply mb-0;
}

.issue-markdown-content :deep(h1),
.issue-markdown-content :deep(h2),
.issue-markdown-content :deep(h3),
.issue-markdown-content :deep(h4),
.issue-markdown-content :deep(h5),
.issue-markdown-content :deep(h6) {
  @apply my-3 font-semibold tracking-normal text-inherit;
}

.issue-markdown-content :deep(h1) {
  @apply text-xl;
}

.issue-markdown-content :deep(h2) {
  @apply text-lg;
}

.issue-markdown-content :deep(h3),
.issue-markdown-content :deep(h4),
.issue-markdown-content :deep(h5),
.issue-markdown-content :deep(h6) {
  @apply text-base;
}

.issue-markdown-content :deep(ul),
.issue-markdown-content :deep(ol) {
  @apply my-2 pl-5;
}

.issue-markdown-content :deep(ul) {
  @apply list-disc;
}

.issue-markdown-content :deep(ol) {
  @apply list-decimal;
}

.issue-markdown-content :deep(li) {
  @apply my-1;
}

.issue-markdown-content :deep(blockquote) {
  @apply my-3 border-l-4 border-primary-300 bg-primary-50/70 px-3 py-2 text-gray-700 dark:border-primary-800 dark:bg-primary-950/30 dark:text-gray-300;
}

.issue-markdown-content :deep(code) {
  @apply rounded bg-gray-100 px-1.5 py-0.5 font-mono text-[0.92em] text-gray-800 dark:bg-dark-700 dark:text-gray-100;
}

.issue-markdown-content :deep(pre) {
  @apply my-3 overflow-x-auto rounded-lg border border-gray-200 bg-gray-950 p-3 text-xs leading-5 text-gray-100 dark:border-dark-600;
}

.issue-markdown-content :deep(pre code) {
  @apply bg-transparent p-0 text-inherit;
}

.issue-markdown-content :deep(a) {
  @apply break-words font-medium text-primary-600 underline underline-offset-2 hover:text-primary-700 dark:text-primary-300 dark:hover:text-primary-200;
}

.issue-markdown-content :deep(table) {
  @apply my-3 block w-full overflow-x-auto rounded-lg border border-gray-200 text-left text-xs dark:border-dark-600;
}

.issue-markdown-content :deep(th),
.issue-markdown-content :deep(td) {
  @apply border-b border-gray-200 px-3 py-2 align-top dark:border-dark-600;
}

.issue-markdown-content :deep(th) {
  @apply bg-gray-50 font-semibold dark:bg-dark-900;
}

.issue-markdown-content :deep(hr) {
  @apply my-4 border-gray-200 dark:border-dark-600;
}
</style>
