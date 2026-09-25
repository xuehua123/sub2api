<template>
  <AppLayout
    ><div class="space-y-4">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 class="text-xl font-semibold">{{ t('userBusiness.title') }}</h1>
          <p class="mt-1 text-sm text-gray-500">
            {{ t('userBusiness.description') }}
          </p>
        </div>
        <div class="flex flex-wrap gap-2">
          <button
            v-for="preset in presets"
            :key="preset"
            class="btn btn-secondary btn-sm"
            @click="setPreset(preset)"
          >
            {{ t('userBusiness.' + preset) }}
          </button>
        </div>
      </div>
      <form
        class="flex flex-wrap items-end gap-3 border-y border-gray-200 py-4 dark:border-dark-700"
        @submit.prevent="applyFilters"
      >
        <label class="w-full text-xs text-gray-500 sm:w-auto"
          >{{ t('userBusiness.start')
          }}<input
            v-model="filters.start_date"
            type="date"
            class="input mt-1"
            required
            :max="filters.end_date"
        /></label>
        <label class="w-full text-xs text-gray-500 sm:w-auto"
          >{{ t('userBusiness.end')
          }}<input
            v-model="filters.end_date"
            type="date"
            class="input mt-1"
            required
            :min="filters.start_date"
            :max="today()"
        /></label>
        <input
          v-model="filters.search"
          class="input min-w-48 flex-1"
          :placeholder="t('userBusiness.search')"
          :aria-label="t('userBusiness.search')"
        />
        <select
          v-model="filters.cost_mode"
          class="input w-44"
          :aria-label="t('userBusiness.costMode')"
          @change="applyFilters"
        >
          <option value="historical">{{ t('userBusiness.historical') }}</option>
          <option value="estimate">{{ t('userBusiness.estimate') }}</option>
        </select>
        <button
          type="button"
          class="btn btn-secondary"
          :disabled="!loaded.start_date"
          @click="fxOpen = true"
        >
          {{ t('userBusiness.fxTitle') }}
        </button>
        <label
          v-if="filters.cost_mode === 'estimate'"
          class="text-xs text-gray-500"
          >{{ t('userBusiness.rate')
          }}<input
            v-model.number="filters.usd_cny"
            type="number"
            min="0.01"
            max="100"
            step="0.0001"
            class="input mt-1 w-32"
            required
        /></label>
        <button class="btn btn-primary" :disabled="loading">
          {{ t('userBusiness.query') }}
        </button>
      </form>
      <div class="flex flex-wrap items-center gap-2">
        <button
          v-for="item in filterOptions"
          :key="item.value"
          class="rounded-md border px-3 py-1.5 text-xs"
          :class="
            filters.filter === item.value
              ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300'
              : 'border-gray-200 text-gray-600 dark:border-dark-600 dark:text-gray-300'
          "
          :aria-pressed="filters.filter === item.value"
          @click="setFilter(item.value)"
        >
          {{ t('userBusiness.' + item.key) }}
        </button>
        <select
          v-model="filters.sort"
          class="input w-40 sm:ml-auto"
          :aria-label="t('userBusiness.sort')"
          @change="applyFilters"
        >
          <option
            v-for="key in ['consumption', 'paid', 'cost', 'profit', 'balance']"
            :key="key"
            :value="key"
          >
            {{ t('userBusiness.' + key) }}
          </option>
        </select>
        <select
          v-model="filters.order"
          class="input w-32"
          :aria-label="t('userBusiness.sort') + ' direction'"
          @change="applyFilters"
        >
          <option value="desc">{{ t('userBusiness.descending') }}</option>
          <option value="asc">{{ t('userBusiness.ascending') }}</option>
        </select>
      </div>
      <p class="text-xs text-gray-500">
        {{ t('userBusiness.basis') }} {{ t('userBusiness.coverage') }}
      </p>
      <div v-if="loading" role="status" class="py-16 text-center text-gray-500">
        {{ t('userBusiness.loading') }}
      </div>
      <p v-else-if="error" role="alert" class="py-12 text-center text-red-600">
        {{ t('userBusiness.failed') }}
        <button class="underline" @click="load">
          {{ t('userBusiness.retry') }}
        </button>
      </p>
      <template v-else-if="report">
        <p class="text-xs text-amber-700 dark:text-amber-300">
          {{
            t(
              report.cost_mode === 'estimate'
                ? 'userBusiness.estimateNotice'
                : 'userBusiness.historicalNotice',
            )
          }}
        </p>
        <div class="flex flex-wrap justify-between gap-2 text-xs text-gray-500">
          <span
            >{{ loaded.start_date }} → {{ loaded.end_date }} ·
            {{ report.timezone }} ·
            {{
              report.cost_mode === 'estimate'
                ? t('userBusiness.estimate') + ' ' + report.usd_cny
                : t('userBusiness.historical')
            }}</span
          ><span
            >{{ t('userBusiness.asOf') }} {{ formatDate(report.as_of) }}</span
          >
        </div>
        <p
          v-if="report.summary.uncertain_count"
          class="border-l-2 border-amber-500 bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:bg-amber-950/20 dark:text-amber-300"
        >
          {{ t('userBusiness.unknownNote') }}
        </p>
        <div
          class="grid grid-cols-2 border-y border-gray-200 dark:border-dark-700 sm:grid-cols-5"
        >
          <div
            v-for="item in summaryItems"
            :key="item.key"
            class="min-w-0 px-3 py-3"
          >
            <p class="text-xs text-gray-500">{{ item.label }}</p>
            <p
              class="mt-1 text-lg font-semibold tabular-nums"
              :class="item.color"
            >
              {{ item.text }}
            </p>
          </div>
        </div>
        <div
          class="overflow-x-auto border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900"
        >
          <table class="w-full min-w-[1200px] text-sm">
            <thead
              class="bg-gray-50 text-left text-xs text-gray-500 dark:bg-dark-800"
            >
              <tr>
                <th
                  class="w-60 max-w-60 sticky left-0 z-10 bg-gray-50 p-3 dark:bg-dark-800"
                >
                  {{ t('userBusiness.rank') }} / {{ t('userBusiness.user') }}
                </th>
                <th class="p-3 text-right">
                  {{ t('userBusiness.paid') }}
                  <p class="font-normal">{{ t('userBusiness.currency') }}</p>
                </th>
                <th class="p-3 text-right">
                  {{ t('userBusiness.consumption') }}
                  <p class="font-normal">{{ t('userBusiness.quota') }}</p>
                </th>
                <th class="p-3 text-right">{{ t('userBusiness.cost') }}</th>
                <th class="p-3 text-right">{{ t('userBusiness.profit') }}</th>
                <th class="border-l p-3 text-right dark:border-dark-700">
                  {{ t('userBusiness.balance') }}
                  <p class="font-normal">{{ t('userBusiness.quota') }}</p>
                </th>
                <th class="p-3">{{ t('userBusiness.cards') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="row in report.items"
                :key="row.id"
                data-testid="business-row"
                class="border-t border-gray-100 dark:border-dark-800"
              >
                <td
                  class="w-60 max-w-60 sticky left-0 z-[1] bg-white p-3 dark:bg-dark-900"
                >
                  <button class="text-left" @click="openDetail(row)">
                    <span class="mr-2 text-xs font-semibold text-gray-400">{{
                      row.rank
                    }}</span
                    ><span
                      class="font-medium text-primary-700 dark:text-primary-300"
                      >{{ row.username || t('userBusiness.emptyName') }}</span
                    ><span class="block break-all text-xs text-gray-500"
                      >{{ row.email }} · #{{ row.id }}</span
                    ><span v-if="row.deleted" class="text-xs text-red-500">{{
                      t('userBusiness.deleted')
                    }}</span>
                  </button>
                </td>
                <td class="p-3 text-right tabular-nums">
                  <strong>{{ money(row.paid) }}</strong
                  ><span
                    v-if="row.uncertain_count"
                    class="ml-1 text-xs text-amber-600"
                    >{{ t('userBusiness.review') }}</span
                  >
                  <p class="text-xs text-gray-500">
                    {{ t('userBusiness.cash') }} {{ money(row.balance_paid) }}
                  </p>
                  <p class="text-xs text-gray-500">
                    {{ t('userBusiness.subscription') }}
                    {{ money(row.subscription_paid) }}
                  </p>
                  <p v-if="row.refund" class="text-xs text-red-500">
                    {{ t('userBusiness.refund') }} {{ money(row.refund) }}
                  </p>
                </td>
                <td class="p-3 text-right tabular-nums">
                  <strong>{{ quota(row.consumption) }}</strong>
                  <p class="text-xs text-gray-500">
                    {{ t('userBusiness.cash') }}
                    {{ quota(row.balance_consumption) }}
                  </p>
                  <p class="text-xs text-gray-500">
                    {{ t('userBusiness.subscription') }}
                    {{ quota(row.subscription_consumption) }}
                  </p>
                </td>
                <td
                  class="p-3 text-right tabular-nums text-amber-700 dark:text-amber-300"
                >
                  {{ money(row.cost) }}
                </td>
                <td
                  class="p-3 text-right tabular-nums"
                  :class="
                    row.profit === null
                      ? 'text-gray-400'
                      : row.profit >= 0
                        ? 'text-emerald-700 dark:text-emerald-300'
                        : 'text-red-600'
                  "
                >
                  {{ money(row.profit) }}
                </td>
                <td
                  class="border-l p-3 text-right tabular-nums dark:border-dark-700"
                >
                  {{ quota(row.balance) }}
                </td>
                <td class="p-3">
                  <button
                    class="text-primary-700 dark:text-primary-300"
                    @click="openDetail(row)"
                  >
                    {{
                      t('userBusiness.cardCount', { count: row.active_count })
                    }}
                  </button>
                </td>
              </tr>
              <tr v-if="!report.items.length">
                <td colspan="7" class="p-12 text-center text-gray-500">
                  {{ t('userBusiness.noData') }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <Pagination
          :page="loaded.page"
          :page-size="loaded.page_size"
          :total="report.total"
          @update:page="changePage"
          @update:page-size="changeSize"
        />
      </template>
      <UserBusinessFX
        :show="fxOpen"
        :start="loaded.start_date"
        :end="loaded.end_date"
        @close="fxOpen = false"
        @saved="load"
      />
      <UserBusinessDetail
        :user="selected"
        :filters="loaded"
        @close="selected = null"
      /></div
  ></AppLayout>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  getBusinessReport,
  type BusinessFilters,
  type BusinessReport,
  type BusinessRow,
} from '@/api/admin/userBusiness'
import AppLayout from '@/components/layout/AppLayout.vue'
import Pagination from '@/components/common/Pagination.vue'
import UserBusinessDetail from '@/components/admin/UserBusinessDetail.vue'
import UserBusinessFX from '@/components/admin/UserBusinessFX.vue'
const { t } = useI18n()
const zone = ref('Asia/Shanghai')
const today = () =>
  new Intl.DateTimeFormat('en-CA', {
    timeZone: zone.value,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).format(new Date())
function shift(d: string, n: number) {
  const x = new Date(d + 'T12:00:00Z')
  x.setUTCDate(x.getUTCDate() + n)
  return x.toISOString().slice(0, 10)
}
const presets = ['today', 'yesterday', 'week', 'month'] as const
const filters = ref<BusinessFilters>({
  start_date: '',
  end_date: '',
  search: '',
  filter: 'used',
  cost_mode: 'historical',
  sort: 'consumption',
  order: 'desc',
  page: 1,
  page_size: 20,
})
const loaded = ref<BusinessFilters>({ ...filters.value })
const report = ref<BusinessReport | null>(null)
const selected = ref<BusinessRow | null>(null)
const fxOpen = ref(false)
const loading = ref(false)
const error = ref(false)
let controller: AbortController | undefined
let generation = 0
const filterOptions = [
  { value: '', key: 'all' },
  { value: 'used', key: 'used' },
  { value: 'paid', key: 'paidOnly' },
  { value: 'loss', key: 'loss' },
  { value: 'active', key: 'active' },
]
function setPreset(p: (typeof presets)[number]) {
  filters.value.end_date = p === 'yesterday' ? shift(today(), -1) : today()
  filters.value.start_date =
    p === 'week'
      ? shift(filters.value.end_date, -6)
      : p === 'month'
        ? shift(filters.value.end_date, -29)
        : filters.value.end_date
  applyFilters()
}
function setFilter(value: string) {
  filters.value.filter = value
  applyFilters()
}
function applyFilters() {
  filters.value.page = 1
  void load()
}
function changePage(value: number) {
  filters.value = { ...loaded.value, page: value }
  void load()
}
function changeSize(value: number) {
  filters.value = { ...loaded.value, page_size: value, page: 1 }
  void load()
}
async function load() {
  const current = ++generation
  controller?.abort()
  controller = new AbortController()
  loading.value = true
  error.value = false
  selected.value = null
  const params = { ...filters.value }
  try {
    const data = await getBusinessReport({ ...params }, controller.signal)
    if (current !== generation) return
    report.value = data
    zone.value = data.timezone
    if (!params.start_date) {
      params.start_date = today()
      params.end_date = today()
    }
    params.usd_cny = data.usd_cny
    params.cost_mode = data.cost_mode ?? params.cost_mode
    loaded.value = params
    filters.value = { ...params }
  } catch {
    if (current === generation) error.value = true
  } finally {
    if (current === generation) loading.value = false
  }
}
function openDetail(row: BusinessRow) {
  selected.value = row
}
const money = (v: number | null) =>
  v === null
    ? t('userBusiness.review')
    : new Intl.NumberFormat('zh-CN', {
        style: 'currency',
        currency: 'CNY',
        minimumFractionDigits: 2,
        maximumFractionDigits: 2,
      }).format(v)
const quota = (v: number) =>
  new Intl.NumberFormat('en-US', { maximumFractionDigits: 4 }).format(v)
const formatDate = (value: string) =>
  new Intl.DateTimeFormat('zh-CN', {
    dateStyle: 'short',
    timeStyle: 'short',
    timeZone: zone.value,
  }).format(new Date(value))
const summaryItems = computed(() => {
  const s = report.value?.summary
  return [
    {
      key: 'paid',
      label: t('userBusiness.paid'),
      text: money(s?.paid ?? 0),
      color: 'text-sky-600',
    },
    {
      key: 'consumption',
      label: t('userBusiness.consumption') + ' · ' + t('userBusiness.quota'),
      text: quota(s?.consumption ?? 0),
      color: 'text-gray-900 dark:text-white',
    },
    {
      key: 'cost',
      label: t('userBusiness.cost'),
      text: money(s ? s.cost : 0),
      color: 'text-amber-600',
    },
    {
      key: 'profit',
      label: t('userBusiness.profit'),
      text: money(s ? s.profit : 0),
      color:
        s?.profit !== null && (s?.profit ?? 0) < 0
          ? 'text-red-600'
          : 'text-emerald-600',
    },
    {
      key: 'users',
      label: t('userBusiness.users'),
      text: String(report.value?.total ?? 0),
      color: 'text-gray-900 dark:text-white',
    },
  ]
})
void load()
onBeforeUnmount(() => {
  generation++
  controller?.abort()
})
</script>
