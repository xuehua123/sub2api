<template>
  <section class="space-y-4" data-testid="upstream-group-catalog">
    <div class="flex flex-wrap items-center gap-3">
      <div class="relative min-w-60 flex-1">
        <Icon
          name="search"
          size="md"
          class="absolute left-3 top-3 text-gray-400"
        /><input
          v-model="filters.search"
          class="input pl-10"
          :placeholder="t('upstreamWorkspace.catalogSearch')"
          :aria-label="t('upstreamWorkspace.catalogSearch')"
        />
      </div>
      <details class="relative">
        <summary class="btn btn-secondary cursor-pointer">
          {{ t('upstreamWorkspace.upstream')
          }}<span v-if="filters.ids.length" class="ml-2">{{
            filters.ids.length
          }}</span>
        </summary>
        <div
          class="absolute right-0 z-20 mt-1 max-h-72 w-64 overflow-auto rounded-md border border-gray-200 bg-white p-2 shadow-lg dark:border-dark-600 dark:bg-dark-800"
        >
          <label
            v-for="item in connections"
            :key="item.id"
            class="flex items-center gap-2 p-2 text-sm"
            ><input v-model="filters.ids" type="checkbox" :value="item.id" />{{
              item.name
            }}</label
          >
        </div>
      </details>
      <select
        v-model="filters.provider"
        class="input w-36"
        :aria-label="t('upstreamWorkspace.provider')"
      >
        <option value="">
          {{ t('admin.upstreamConnections.allProviders') }}
        </option>
        <option v-for="provider in providers" :key="provider" :value="provider">
          {{ provider }}
        </option>
      </select>
      <button
        class="btn btn-secondary"
        :title="t('common.refresh')"
        :aria-label="t('common.refresh')"
        @click="load"
      >
        <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
      </button>
    </div>
    <div class="flex flex-wrap items-center gap-2">
      <select
        v-model="filters.tag"
        class="input w-36"
        :aria-label="t('upstreamWorkspace.tags')"
      >
        <option value="">{{ t('upstreamWorkspace.allTags') }}</option>
        <option v-for="tag in result.tags" :key="tag" :value="tag">
          {{ tag }}
        </option>
      </select>
      <select
        v-model="filters.binding"
        class="input w-40"
        :aria-label="t('upstreamWorkspace.binding')"
      >
        <option value="">{{ t('upstreamWorkspace.allBindings') }}</option>
        <option value="bound">{{ t('upstreamWorkspace.bound') }}</option>
        <option value="unbound">{{ t('upstreamWorkspace.unbound') }}</option>
      </select>
      <select
        v-model="filters.freshness"
        class="input w-40"
        :aria-label="t('upstreamWorkspace.freshness')"
      >
        <option value="">{{ t('upstreamWorkspace.allFreshness') }}</option>
        <option
          v-for="value in ['fresh', 'stale', 'unknown', 'error']"
          :key="value"
          :value="value"
        >
          {{ t('upstreamWorkspace.' + value) }}
        </option>
      </select>
      <div class="flex items-center gap-2">
      <input
        v-model="filters.min"
        type="number"
        min="0"
        step="any"
        class="input w-28"
        :placeholder="t('upstreamWorkspace.rateMin')"
        :aria-label="t('upstreamWorkspace.rateMin')"
      />
      <span class="text-gray-400">–</span
      ><input
        v-model="filters.max"
        type="number"
        min="0"
        step="any"
        class="input w-28"
        :placeholder="t('upstreamWorkspace.rateMax')"
        :aria-label="t('upstreamWorkspace.rateMax')"
      />
      </div>
      <label class="flex items-center gap-2 px-2 text-sm"
        ><input v-model="filters.favorites" type="checkbox" />{{
          t('upstreamWorkspace.favorites')
        }}</label
      >
      <button class="btn btn-secondary" @click="reset">
        {{ t('upstreamWorkspace.reset') }}
      </button>
    </div>
    <div
      class="flex flex-wrap items-center justify-between gap-3 border-y border-gray-200 py-3 dark:border-dark-700"
    >
      <span class="text-sm text-gray-500">{{
        t('upstreamWorkspace.catalogCount', { count: result.total })
      }}</span>
      <div class="flex flex-wrap items-center gap-2">
        <div
          class="inline-flex rounded-md border border-gray-200 p-0.5 dark:border-dark-600"
        >
          <button
            v-for="mode in [false, true]"
            :key="String(mode)"
            class="rounded px-3 py-1.5 text-sm"
            :class="
              filters.grouped === mode
                ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300'
                : 'text-gray-500'
            "
            :aria-pressed="filters.grouped === mode"
            @click="filters.grouped = mode"
          >
            {{
              t(mode ? 'upstreamWorkspace.grouped' : 'upstreamWorkspace.flat')
            }}
          </button>
        </div>
        <select
          v-model="filters.sort"
          class="input w-44"
          :aria-label="t('upstreamWorkspace.sort')"
        >
          <option v-for="(label, key) in sorts" :key="key" :value="key">
            {{ t('upstreamWorkspace.' + label) }}
          </option>
        </select>
      </div>
    </div>
    <p class="text-xs text-gray-500">{{ t('upstreamWorkspace.scope') }}</p>
    <div
      v-if="selected.length"
      class="flex flex-wrap items-center gap-3 bg-primary-50 px-3 py-2 dark:bg-primary-900/20"
    >
      <span class="text-sm">{{
        t('upstreamWorkspace.selected', { count: selected.length })
      }}</span>
      <button
        class="btn btn-secondary btn-sm"
        :disabled="saving"
        @click="openTags('add')"
      >
        {{ t('upstreamWorkspace.addTags') }}
      </button>
      <button
        class="btn btn-secondary btn-sm"
        :disabled="saving"
        @click="openTags('remove')"
      >
        {{ t('upstreamWorkspace.removeTags') }}
      </button>
    </div>
    <p v-if="error" role="alert" class="text-sm text-red-600">
      {{ error }}
      <button class="underline" @click="load">
        {{ t('upstreamWorkspace.retry') }}
      </button>
    </p>
    <div v-if="loading" class="py-12 text-center text-gray-500" role="status">
      {{ t('upstreamWorkspace.busy') }}
    </div>
    <div v-else-if="!error" class="overflow-x-auto bg-white dark:bg-dark-900">
      <table class="w-full min-w-[900px] text-sm">
        <thead
          class="bg-gray-50 text-left text-xs text-gray-500 dark:bg-dark-800"
        >
          <tr>
            <th class="w-10 p-3">
              <input
                type="checkbox"
                :checked="
                  result.items.length > 0 &&
                  selected.length === result.items.length
                "
                :aria-label="t('upstreamWorkspace.selectAll')"
                @change="
                  selectPage(($event.target as HTMLInputElement).checked)
                "
              />
            </th>
            <th class="p-3">{{ t('upstreamWorkspace.name') }}</th>
            <th class="p-3">{{ t('upstreamWorkspace.upstream') }}</th>
            <th class="p-3">{{ t('upstreamWorkspace.tags') }}</th>
            <th class="p-3 text-right">{{ t('upstreamWorkspace.rate') }}</th>
            <th class="p-3 text-right">
              {{ t('upstreamWorkspace.accounts') }}
            </th>
            <th class="p-3">{{ t('upstreamWorkspace.freshness') }}</th>
            <th class="p-3">{{ t('upstreamWorkspace.updated') }}</th>
          </tr>
        </thead>
        <tbody>
          <template v-for="(row, index) in result.items" :key="keyOf(row)">
            <tr
              v-if="
                filters.grouped &&
                (index === 0 ||
                  result.items[index - 1].connection_id !== row.connection_id)
              "
              class="bg-gray-50 dark:bg-dark-800"
            >
              <td colspan="8" class="px-3 py-2 font-medium">
                {{ row.connection_name }}
                <span class="ml-2 text-xs font-normal text-gray-500">{{
                  row.provider
                }}</span>
              </td>
            </tr>
            <tr
              class="border-b border-gray-100 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-800"
              data-testid="catalog-row"
            >
              <td class="p-3">
                <input
                  v-model="selected"
                  :value="keyOf(row)"
                  type="checkbox"
                  :aria-label="
                    t('upstreamWorkspace.selectRow') + ' ' + row.name
                  "
                />
              </td>
              <td class="p-3">
                <div class="flex items-center gap-2">
                  <button
                    :disabled="saving"
                    :title="t('upstreamWorkspace.favorite')"
                    :aria-label="
                      t('upstreamWorkspace.favorite') + ' ' + row.name
                    "
                    :aria-pressed="row.favorite"
                    class="p-1"
                    :class="row.favorite ? 'text-amber-500' : 'text-gray-400'"
                    @click="favorite(row)"
                  >
                    {{ row.favorite ? '★' : '☆' }}
                  </button>
                  <div>
                    <button
                      class="font-medium text-primary-700 dark:text-primary-300"
                      @click="$emit('details', row.connection_id)"
                    >
                      {{ row.name }}
                    </button>
                    <p class="text-xs text-gray-400">
                      {{ row.remote_id || '—' }}
                    </p>
                  </div>
                </div>
              </td>
              <td class="p-3">
                <button
                  class="text-left"
                  :title="row.management_base_url"
                  @click="$emit('details', row.connection_id)"
                >
                  {{ row.connection_name
                  }}<span class="block text-xs text-gray-500">{{
                    row.provider
                  }}</span>
                </button>
              </td>
              <td class="max-w-60 p-3">
                <div class="flex flex-wrap items-center gap-1">
                  <span
                    v-for="tag in row.tags"
                    :key="tag"
                    class="rounded bg-primary-50 px-2 py-0.5 text-xs text-primary-700 dark:bg-primary-900/20 dark:text-primary-300"
                    >{{ tag }}</span
                  ><button
                    :title="t('upstreamWorkspace.addTags')"
                    :aria-label="
                      t('upstreamWorkspace.addTags') + ' ' + row.name
                    "
                    class="p-1 text-gray-400 hover:text-primary-600"
                    @click="openRowTags(row)"
                  >
                    <Icon name="plus" size="sm" />
                  </button>
                </div>
              </td>
              <td
                class="p-3 text-right font-medium tabular-nums"
                :title="row.source + ' / ' + row.confidence"
              >
                {{
                  row.rate_multiplier === null
                    ? t('upstreamWorkspace.unknown')
                    : row.rate_multiplier + '×'
                }}
              </td>
              <td class="p-3 text-right tabular-nums">
                <button
                  v-if="row.binding_count"
                  class="text-primary-600"
                  @click="$emit('details', row.connection_id)"
                >
                  {{ row.binding_count }}</button
                ><span v-else class="text-gray-400">0</span>
              </td>
              <td class="p-3">
                <span class="inline-flex items-center gap-1.5 text-xs"
                  ><span
                    class="h-1.5 w-1.5 rounded-full"
                    :class="
                      row.freshness === 'fresh'
                        ? 'bg-emerald-500'
                        : row.freshness === 'unknown'
                          ? 'bg-gray-400'
                          : 'bg-amber-500'
                    "
                  />{{ t('upstreamWorkspace.' + row.freshness) }}</span
                >
              </td>
              <td class="whitespace-nowrap p-3 text-xs text-gray-500">
                {{ row.observed_at ? formatDateTime(row.observed_at) : '—' }}
              </td>
            </tr>
          </template>
          <tr v-if="!result.items.length">
            <td colspan="8" class="py-12 text-center text-gray-500">
              {{ t('upstreamWorkspace.noGroups') }}
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <Pagination
      v-if="result.total > 0"
      :page="page"
      :page-size="pageSize"
      :total="result.total"
      @update:page="changePage"
      @update:page-size="changePageSize"
    />
    <BaseDialog
      :show="tagDialog"
      :title="
        t(
          tagMode === 'add'
            ? 'upstreamWorkspace.addTags'
            : 'upstreamWorkspace.removeTags',
        )
      "
      @close="tagDialog = false"
    >
      <form id="catalog-tag-form" @submit.prevent="saveTags">
        <input
          v-model="tagInput"
          class="input"
          :placeholder="t('upstreamWorkspace.tagInput')"
          :aria-label="t('upstreamWorkspace.tagInput')"
          maxlength="660"
          required
        />
        <p v-if="tagError" class="mt-2 text-sm text-red-600">{{ tagError }}</p>
      </form>
      <template #footer
        ><div class="flex justify-end gap-2">
          <button class="btn btn-secondary" @click="tagDialog = false">
            {{ t('upstreamWorkspace.cancel') }}</button
          ><button
            class="btn btn-primary"
            form="catalog-tag-form"
            type="submit"
            :disabled="saving"
          >
            {{ t('upstreamWorkspace.save') }}
          </button>
        </div></template
      >
    </BaseDialog>
  </section>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { formatDateTime } from '@/utils/format'
import {
  getGroupCatalog,
  annotateGroups,
  type CatalogGroup,
  type CatalogResult,
} from '@/api/admin/upstreamCatalog'
const props = defineProps<{
  connections: { id: number; name: string; provider: string }[]
  connectionId?: number
}>()
defineEmits<{ details: [id: number] }>()
const { t } = useI18n()
const filters = reactive({
  search: '',
  ids: [] as number[],
  provider: '',
  tag: '',
  binding: '',
  freshness: '',
  min: '' as string | number,
  max: '' as string | number,
  favorites: false,
  grouped: false,
  sort: 'name_asc',
})
const defaults = JSON.stringify(filters)
try {
  const saved = JSON.parse(
    localStorage.getItem('upstream-catalog-filters') || '{}',
  )
  for (const key of [
    'search',
    'provider',
    'tag',
    'binding',
    'freshness',
    'sort',
  ] as const)
    if (typeof saved[key] === 'string') filters[key] = saved[key]
  for (const key of ['favorites', 'grouped'] as const)
    if (typeof saved[key] === 'boolean') filters[key] = saved[key]
  if (Array.isArray(saved.ids))
    filters.ids = saved.ids
      .filter(
        (id: unknown) =>
          typeof id === 'number' && Number.isSafeInteger(id) && id > 0,
      )
      .slice(0, 200)
  for (const key of ['min', 'max'] as const)
    if (
      typeof saved[key] === 'number' &&
      Number.isFinite(saved[key]) &&
      saved[key] >= 0
    )
      filters[key] = saved[key]
} catch {
  /* Invalid or unavailable storage must not block the catalog. */
}
const providers = computed(() =>
  [...new Set(props.connections.map((x) => x.provider))].sort(),
)
const sorts = {
  name_asc: 'nameAsc',
  name_desc: 'nameDesc',
  rate_asc: 'rateAsc',
  rate_desc: 'rateDesc',
  bindings_desc: 'bindingsDesc',
  observed_desc: 'observedDesc',
  connection_asc: 'connectionAsc',
}
const result = ref<CatalogResult>({
  items: [],
  total: 0,
  page: 1,
  page_size: 20,
  tags: [],
})
const page = ref(1),
  pageSize = ref(20),
  loading = ref(false),
  error = ref(''),
  selected = ref<string[]>([]),
  saving = ref(false)
const tagDialog = ref(false),
  tagInput = ref(''),
  tagMode = ref<'add' | 'remove'>('add'),
  tagError = ref('')
let controller: AbortController | undefined,
  timer: ReturnType<typeof setTimeout> | undefined,
  generation = 0
const keyOf = (r: CatalogGroup) =>
  JSON.stringify([r.connection_id, r.remote_key])
async function load() {
  clearTimeout(timer)
  controller?.abort()
  controller = new AbortController()
  const current = ++generation
  loading.value = true
  error.value = ''
  selected.value = []
  try {
    const data = await getGroupCatalog(
      {
        page: page.value,
        page_size: pageSize.value,
        search: filters.search,
        connection_ids: filters.ids.join(','),
        provider: filters.provider,
        tag: filters.tag,
        binding: filters.binding,
        freshness: filters.freshness,
        sort: filters.sort,
        favorites: filters.favorites,
        group_by_connection: filters.grouped,
        min_rate: filters.min === '' ? undefined : Number(filters.min),
        max_rate: filters.max === '' ? undefined : Number(filters.max),
      },
      controller.signal,
    )
    if (current === generation) result.value = data
  } catch {
    if (current === generation) error.value = t('upstreamWorkspace.loadError')
  } finally {
    if (current === generation) loading.value = false
  }
}
function reset() {
  Object.assign(filters, JSON.parse(defaults))
}
function openRowTags(row: CatalogGroup) { selected.value=[keyOf(row)];openTags('add') }
function changePage(value: number) { page.value=value;void load() }
function changePageSize(value: number) { pageSize.value=value;page.value=1;void load() }
function selectPage(value: boolean) {
  selected.value = value ? result.value.items.map(keyOf) : []
}
function openTags(mode: 'add' | 'remove') {
  tagMode.value = mode
  tagInput.value = ''
  tagError.value = ''
  tagDialog.value = true
}
async function saveTags() {
  const tags = [
    ...new Set(
      tagInput.value
        .split(/[,，]/)
        .map((x) => x.trim())
        .filter(Boolean),
    ),
  ]
  if (!tags.length) return
  saving.value = true
  try {
    await annotateGroups(
      result.value.items
        .filter((x) => selected.value.includes(keyOf(x)))
        .map((x) => ({
          connection_id: x.connection_id,
          remote_key: x.remote_key,
        })),
      tagMode.value === 'add' ? { add_tags: tags } : { remove_tags: tags },
    )
    tagDialog.value = false
    await load()
  } catch {
    tagError.value = t('upstreamWorkspace.annotationError')
  } finally {
    saving.value = false
  }
}
async function favorite(row: CatalogGroup) {
  saving.value = true
  try {
    await annotateGroups(
      [{ connection_id: row.connection_id, remote_key: row.remote_key }],
      { favorite: !row.favorite },
    )
    await load()
  } catch {
    error.value = t('upstreamWorkspace.annotationError')
  } finally {
    saving.value = false
  }
}
watch(filters, () => {
  try {
    localStorage.setItem('upstream-catalog-filters', JSON.stringify(filters))
  } catch {
    /* Optional preferences. */
  }
  page.value = 1
  clearTimeout(timer)
  generation++
  controller?.abort()
  loading.value = true
  timer = setTimeout(() => void load(), 250)
})
watch(
  () => props.connectionId,
  (id) => {
    filters.ids = id ? [id] : []
  },
)
onMounted(() => {
  if (props.connectionId) filters.ids = [props.connectionId]
  void load()
})
onBeforeUnmount(() => {
  generation++
  clearTimeout(timer)
  controller?.abort()
})
</script>
