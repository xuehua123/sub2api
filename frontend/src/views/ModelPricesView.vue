<template>
  <PricePortalLayout>
    <div class="ppx-price-grid">
      <aside class="ppx-group-rail">
        <ModelPriceGroupPicker
          :groups="groups"
          :selectedID="selectedGroupID"
          :busy="busy"
          @select="selectedGroupID = $event"
        />
      </aside>
      <main id="price-main" class="price-workspace">
        <div class="price-breadcrumb">
          <a href="/home">首页</a><span>/</span><span>模型与价格</span
          ><span class="price-catalog-marker"><i></i> OFFICIAL CATALOG</span>
        </div>
        <header class="price-page-heading">
          <div>
            <p class="price-eyebrow">MODEL PRICING</p>
            <h1 class="price-title">模型价格</h1>
            <p class="price-page-description">
              {{ selectedGroup?.name || '选择分组' }} ·
              {{ filteredModels.length }} 个模型 · {{ manage && isAdmin ? '展示价格 · CNY' : '官方原价 · USD' }}
            </p>
          </div>
          <div class="flex items-center gap-2">
            <button
              class="price-icon self-end"
              title="重置筛选"
              aria-label="重置筛选"
              @click="resetFilters"
            >
              <Icon name="x" size="md" />
            </button>

            <div
              v-if="isAdmin"
              class="price-tabs"
              role="tablist"
              aria-label="价格工作区"
            >
              <button
                role="tab"
                :aria-selected="!manage"
                :class="{ active: !manage }"
                @click="manage = false"
              >
                价格查询</button
              ><button
                role="tab"
                data-testid="price-manage-tab"
                :aria-selected="manage"
                :class="{ active: manage }"
                @click="manage = true"
              >
                价格管理
              </button>
            </div>
            <button
              class="price-icon"
              title="刷新价格"
              aria-label="刷新价格"
              :disabled="loading || busy"
              @click="loadPrices"
            >
              <Icon
                name="refresh"
                size="md"
                :class="{ 'animate-spin': loading }"
              />
            </button>
          </div>
        </header>

        <ModelPriceOperations
          v-if="isAdmin && manage"
          :response="response"
          :busy="busy"
          :last-action="lastAction"
          :save-settings="saveSettings"
          @sync="syncCatalog"
        />
        <button
          type="button"
          class="btn btn-secondary btn-sm mb-2 md:hidden"
          :aria-expanded="advancedFilters"
          @click="advancedFilters = !advancedFilters"
        >
          {{ advancedFilters ? '收起筛选' : '更多筛选'
          }}{{ platform || billing || source || priceState ? ' · 已筛选' : '' }}
        </button>
        <section class="price-filters" aria-label="价格筛选">
          <label class="price-search"
            >模型<input
              v-model="search"
              class="input"
              placeholder="搜索模型名称或供应商"
              data-testid="model-search"
          /></label>
          <label class="advanced-filter" :class="{ 'is-open': advancedFilters }"
            >平台<select
              v-model="platform"
              class="input"
              data-testid="platform-filter"
            >
              <option value="">全部平台</option>
              <option v-for="p in platforms" :key="p" :value="p">
                {{ p }}
              </option>
            </select></label
          >
          <label class="advanced-filter" :class="{ 'is-open': advancedFilters }"
            >计费单位<select
              v-model="billing"
              class="input"
              data-testid="billing-filter"
            >
              <option value="">全部计费单位</option>
              <option value="token">Token</option>
              <option value="per_request">每次</option>
              <option value="image">图片 / 张</option>
              <option value="video">视频 / 秒</option>
              <option value="unknown">单位待确认</option>
            </select></label
          >
          <label class="advanced-filter" :class="{ 'is-open': advancedFilters }"
            >价格状态<select
              v-model="priceState"
              class="input"
              data-testid="price-state-filter"
            >
              <option value="">全部状态</option>
              <option value="normal">信息完整</option>
              <option value="missing">缺少价格</option>
              <option value="baseline">缺少官方原价</option>
              <option value="route">未提供渠道来源</option>
              <option value="zero">含零价项目</option>
            </select></label
          >
          <label class="advanced-filter" :class="{ 'is-open': advancedFilters }"
            >价格来源<select
              v-model="source"
              class="input"
              data-testid="price-source-filter"
            >
              <option value="">全部来源</option>
              <option v-for="s in sources" :key="s" :value="s">
                {{ sourceLabel(s) }}
              </option>
            </select></label
          >
          <label class="advanced-filter" :class="{ 'is-open': advancedFilters }"
            >排序<select v-model="sort" class="input" data-testid="price-sort">
              <option value="name">模型名称</option>
              <option value="input">官方输入价升序</option>
              <option value="output">官方输出价升序</option>
              <option value="fixed">官方固定价升序</option>
              <option value="issues">异常优先</option>
            </select></label
          >
        </section>
        <div
          class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-200 py-3 text-xs dark:border-dark-700"
        >
          <div class="flex flex-wrap gap-4">
            <label
              ><input v-model="showBaseline" type="checkbox" />
              展开价格明细</label
            >
          </div>
          <div class="flex items-center gap-2">
            <button
              class="price-icon"
              title="复制模型名"
              aria-label="复制模型名"
              :disabled="!filteredModels.length || loading || !!error"
              @click="copy(filteredModels.map((m) => m.name).join('\n'))"
            >
              <Icon name="copy" size="sm" /></button
            ><button
              class="btn btn-secondary btn-sm"
              :disabled="!filteredModels.length || loading || !!error"
              @click="openCalculator(filteredModels[0])"
            >
              费用试算
            </button>
          </div>
        </div>
        <div
          v-if="manage && isAdmin"
          class="space-y-3 border-b border-gray-200 py-3 dark:border-dark-700"
        >
          <div class="flex flex-wrap gap-4 text-xs">
            <label
              ><input
                v-model="includeCatalog"
                :disabled="busy"
                type="checkbox"
              />
              补充参考目录</label
            ><label
              ><input
                v-model="showHiddenGroups"
                :disabled="busy"
                type="checkbox"
              />
              显示隐藏分组</label
            ><label
              ><input
                v-model="showHiddenModels"
                :disabled="busy"
                type="checkbox"
              />
              显示隐藏模型</label
            ><label
              ><input
                v-model="hiddenOnly"
                data-testid="model-hidden-only-filter"
                type="checkbox"
              />
              只看隐藏模型</label
            >
          </div>
          <details>
            <summary class="cursor-pointer text-sm">分组可见性管理</summary>
            <div class="mt-3 grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
              <label
                v-for="g in groups"
                :key="g.id"
                class="flex items-center gap-2 text-xs"
                ><input
                  type="checkbox"
                  :checked="selectedGroups.includes(g.id)"
                  :disabled="busy"
                  :data-testid="'group-select-' + g.id"
                  @change="toggleGroup(g.id)"
                /><span class="break-words"
                  >{{ g.name }} {{ g.hidden ? '· 已隐藏' : '' }}</span
                ></label
              >
            </div>
            <div
              class="mt-3 flex flex-wrap items-center gap-2"
              data-testid="group-bulk-panel"
            >
              <span class="text-xs">已选 {{ selectedGroups.length }} 个</span
              ><button
                class="btn btn-secondary btn-sm"
                :disabled="busy || !selectedGroups.length"
                data-testid="group-bulk-hide"
                @click="confirmGroups(true)"
              >
                隐藏所选分组</button
              ><button
                class="btn btn-secondary btn-sm"
                :disabled="busy || !selectedGroups.length"
                data-testid="group-bulk-restore"
                @click="confirmGroups(false)"
              >
                恢复所选分组
              </button>
            </div>
          </details>
          <div
            v-if="selectedGroup"
            class="flex flex-wrap items-center gap-2"
            data-testid="model-bulk-panel"
          >
            <label class="mr-2 text-xs"
              ><input
                type="checkbox"
                :checked="allPageSelected"
                :disabled="busy || loading"
                @change="selectPage"
              />
              本页全选</label
            ><span class="text-xs">{{
              selectedModels.length
                ? '已选 ' + selectedModels.length + ' 个模型'
                : '勾选模型后批量隐藏或恢复'
            }}</span
            ><button
              class="btn btn-secondary btn-sm"
              :disabled="busy || loading || !selectedModels.length"
              data-testid="model-bulk-hide"
              @click="confirmModels(true)"
            >
              隐藏所选</button
            ><button
              class="btn btn-secondary btn-sm"
              :disabled="busy || loading || !selectedModels.length"
              data-testid="model-bulk-restore"
              @click="confirmModels(false)"
            >
              恢复所选</button
            ><button
              class="price-icon"
              :disabled="busy"
              title="清空选择"
              aria-label="清空选择"
              @click="selectedModels = []"
            >
              <Icon name="x" size="sm" />
            </button>
          </div>
        </div>
        <p class="py-3 text-xs text-gray-500">
          {{manage && isAdmin ? '管理模式展示渠道、分组及自定义展示价，保存后在此核对。价格查询模式始终使用官方原价。' : '显示已同步官方目录的美元原价，不乘分组倍率或套餐折扣。人民币试算仅作汇率换算；目录缺失时不以渠道价替代。'}}
        </p>
        <div v-if="error" role="alert" class="py-8 text-center">
          <p>{{ error }}</p>
          <button class="btn btn-secondary mt-3" @click="loadPrices">
            重试
          </button>
        </div>
        <div
          v-else-if="loading"
          role="status"
          class="py-12 text-center text-sm text-gray-500"
        >
          正在加载价格…
        </div>
        <p v-else-if="!selectedGroup" class="py-6 text-sm text-gray-500">
          从上方选择分组查看模型价格。
        </p>
        <p
          v-else-if="!filteredModels.length"
          class="py-12 text-center text-sm text-gray-500"
        >
          没有匹配的模型<button
            class="ml-2 text-primary-600"
            @click="resetFilters"
          >
            清除筛选
          </button>
        </p>
        <section v-else aria-label="模型价格列表" class="price-catalog">
          <div class="price-catalog-heading">
            <span>模型 / MODEL</span><span>{{manage && isAdmin ? '展示价格 / CNY' : '官方单价 / USD'}}</span
            ><span>价格来源</span><span></span>
          </div>
          <ModelPriceItem
            v-for="m in pageModels"
            :key="modelKey(m)"
            :model="m"
            :manage="manage && isAdmin"
            :basis="manage && isAdmin ? 'display' : 'catalog'"
            :selected="selectedModels.includes(m.name)"
            :busy="busy"
            :show-baseline="showBaseline"
            :show-discount="false"
            :exchange-rate="response?.usd_cny_rate || 0"
            @copy="copy"
            @select="toggleModel(m.name)"
            @edit="openEditor"
            @estimate="openCalculator"
          />
          <nav
            class="flex flex-wrap items-center justify-between gap-3 border-t border-gray-200 py-4 text-xs dark:border-dark-700"
            aria-label="价格分页"
          >
            <span
              >共 {{ filteredModels.length }} 个模型 · 第 {{ page }} /
              {{ pages }} 页</span
            >
            <div class="flex gap-2">
              <button
                class="price-icon"
                title="上一页"
                aria-label="上一页"
                :disabled="page <= 1"
                @click="page--"
              >
                <Icon name="chevronLeft" size="sm" /></button
              ><button
                class="price-icon"
                title="下一页"
                aria-label="下一页"
                :disabled="page >= pages"
                @click="page++"
              >
                <Icon name="chevronRight" size="sm" />
              </button>
            </div>
          </nav>
        </section>
        <BaseDialog
          :show="calculatorOpen"
          title="费用试算"
          width="wide"
          @close="calculatorOpen = false"
          ><ModelPriceCalculator
            v-if="calculatorOpen"
            :models="filteredModels"
            :initial-model="calculatorKey"
            :basis="manage && isAdmin ? 'display' : 'catalog'"
        /></BaseDialog>
        <ModelPriceEditor
          v-if="editor"
          :model="editor.model"
          :groupID="editor.groupID"
          :group-name="editor.groupName"
          :busy="busy"
          @close="editor = null"
          @save="saveEditor"
        />
        <BaseDialog
          :show="!!confirmation"
          title="确认可见性变更"
          width="narrow"
          :show-close-button="!busy"
          :close-on-escape="!busy"
          @close="!busy && (confirmation = null)"
          ><p class="text-sm">
            {{ confirmation?.label }}？仅影响价格页展示，不改变调用权限。
          </p>
          <ul class="mt-3 max-h-48 overflow-auto text-xs">
            <li
              v-for="name in confirmation?.names"
              :key="name"
              class="break-all py-1"
            >
              {{ name }}
            </li>
          </ul>
          <template #footer
            ><div class="flex justify-end gap-2">
              <button
                class="btn btn-secondary"
                :disabled="busy"
                @click="confirmation = null"
              >
                取消</button
              ><button
                class="btn btn-primary"
                :disabled="busy"
                data-testid="confirm-visibility"
                @click="applyVisibility"
              >
                {{ busy ? '处理中…' : '确认' }}
              </button>
            </div></template
          ></BaseDialog
        >
      </main>
    </div>
  </PricePortalLayout>
</template>
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type {
  ModelPriceModel,
  UpdateModelPriceCustomPriceRequest,
} from '@/api/modelPrices'
import PricePortalLayout from '@/components/model-price/PricePortalLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import ModelPriceItem from '@/components/model-price/ModelPriceItem.vue'
import ModelPriceGroupPicker from '@/components/model-price/ModelPriceGroupPicker.vue'
import ModelPriceCalculator from '@/components/model-price/ModelPriceCalculator.vue'
import ModelPriceOperations from '@/components/model-price/ModelPriceOperations.vue'
import ModelPriceEditor from '@/components/model-price/ModelPriceEditor.vue'
import { useModelPrices } from '@/components/model-price/useModelPrices'
import { catalogModel } from '@/components/model-price/catalogPricing'
import {
  kind,
  sourceLabel,
  issues,
  comparePrices,
  modelKey,
  type PriceSort,
} from '@/components/model-price/pricing'
import '@/components/model-price/model-prices.css'
const {
  isAdmin,
  response,
  selectedGroupID,
  selectedGroup,
  groups,
  models: salesModels,
  loading,
  busy,
  error,
  lastAction,
  includeCatalog,
  showHiddenGroups,
  showHiddenModels,
  loadPrices,
  setGroupsHidden,
  setModelsHidden,
  savePrice,
  syncCatalog,
  saveSettings,
  copy,
} = useModelPrices()
const models = computed(() =>
  manage.value && isAdmin.value ? salesModels.value : salesModels.value.map((m) =>
    catalogModel(m, response.value?.usd_cny_rate || 0),
  ),
)
const advancedFilters = ref(false)
const manage = ref(false),
  search = ref(''),
  platform = ref(''),
  billing = ref(''),
  source = ref(''),
  priceState = ref(''),
  sort = ref<PriceSort>('name')
const showBaseline = ref(false),
  hiddenOnly = ref(false),
  page = ref(1)
const selectedGroups = ref<number[]>([]),
  selectedModels = ref<string[]>([])
const platforms = computed(() =>
  [...new Set(models.value.map((m) => m.platform))].sort(),
)
const sources = computed(() =>
  [...new Set(models.value.map((m) => m.pricing_source))].sort(),
)
const filteredModels = computed(() =>
  models.value
    .filter((m) => {
      if (m.hidden && (!isAdmin.value || !manage.value || !showHiddenModels.value)) return false
      if (hiddenOnly.value && !m.hidden) return false
      if (platform.value && m.platform !== platform.value) return false
      if (billing.value && kind(m) !== billing.value) return false
      if (source.value && m.pricing_source !== source.value) return false
      const problems = issues(m)
      if (priceState.value === 'normal' && problems.length) return false
      if (
        priceState.value &&
        priceState.value !== 'normal' &&
        !problems.includes(priceState.value)
      )
        return false
      return (
        !search.value.trim() ||
        (m.name + ' ' + m.provider + ' ' + m.platform)
          .toLowerCase()
          .includes(search.value.trim().toLowerCase())
      )
    })
    .sort((a, b) => comparePrices(a, b, sort.value)),
)
const pages = computed(() =>
  Math.max(1, Math.ceil(filteredModels.value.length / 30)),
)
const pageModels = computed(() =>
  filteredModels.value.slice((page.value - 1) * 30, page.value * 30),
)
const allPageSelected = computed(
  () =>
    pageModels.value.length > 0 &&
    pageModels.value.every((m) => selectedModels.value.includes(m.name)),
)
const calculatorOpen = ref(false),
  calculatorKey = ref('')
const editor = ref<{
  model: ModelPriceModel
  groupID: number
  groupName: string
} | null>(null)
const confirmation = ref<{
  type: 'groups' | 'models'
  ids: number[]
  names: string[]
  hidden: boolean
  label: string
  groupID?: number
} | null>(null)
watch(loading, (value) => {
  if (value) calculatorOpen.value = false
})
watch(filteredModels, () => {
  page.value = 1
  selectedModels.value = []
})
watch(groups, () => {
  selectedGroups.value = selectedGroups.value.filter((id) =>
    groups.value.some((g) => g.id === id),
  )
})
watch(hiddenOnly, (v) => {
  if (v) showHiddenModels.value = true
})
watch(showHiddenGroups, (v) => {
  if(!v && selectedGroup.value?.hidden) selectedGroupID.value=undefined
})
watch(showHiddenModels, (v) => {
  if (!v) hiddenOnly.value = false
})
watch(selectedGroupID, () => {
  resetFilters()
  selectedModels.value = []
  editor.value = null
  calculatorOpen.value = false
  confirmation.value = null
})
watch(manage, () => { resetFilters(); calculatorOpen.value=false; editor.value=null; confirmation.value=null; if(!manage.value){showHiddenGroups.value=false;showHiddenModels.value=false;includeCatalog.value=false} })
function resetFilters() {
  search.value = ''
  platform.value = ''
  billing.value = ''
  source.value = ''
  priceState.value = ''
  sort.value = 'name'
  hiddenOnly.value = false
  page.value = 1
  selectedModels.value = []
}
function toggleModel(name: string) {
  selectedModels.value = selectedModels.value.includes(name)
    ? selectedModels.value.filter((x) => x !== name)
    : [...selectedModels.value, name]
}
function toggleGroup(id: number) {
  selectedGroups.value = selectedGroups.value.includes(id)
    ? selectedGroups.value.filter((x) => x !== id)
    : [...selectedGroups.value, id]
}
function selectPage() {
  selectedModels.value = allPageSelected.value
    ? []
    : pageModels.value.map((m) => m.name)
}
function openCalculator(m: ModelPriceModel) {
  calculatorKey.value = modelKey(m)
  calculatorOpen.value = true
}
function openEditor(m: ModelPriceModel) {
  if (isAdmin.value && selectedGroup.value)
    editor.value = {
      model:
        salesModels.value.find((item) => modelKey(item) === modelKey(m)) || m,
      groupID: selectedGroup.value.id,
      groupName: selectedGroup.value.name,
    }
}
async function saveEditor(body: UpdateModelPriceCustomPriceRequest) {
  if (await savePrice(body)) editor.value = null
}
function confirmGroups(hidden: boolean) {
  confirmation.value = {
    type: 'groups',
    ids: [...selectedGroups.value],
    names: groups.value
      .filter((g) => selectedGroups.value.includes(g.id))
      .map((g) => g.name),
    hidden,
    label: (hidden ? '隐藏' : '恢复') + '所选分组',
  }
}
function confirmModels(hidden: boolean) {
  confirmation.value = {
    type: 'models',
    ids: [],
    names: [...selectedModels.value],
    hidden,
    groupID: selectedGroupID.value,
    label: (hidden ? '隐藏' : '恢复') + '所选模型',
  }
}
async function applyVisibility() {
  const c = confirmation.value
  if (!c || busy.value) return
  if (c.type === 'models' && c.groupID !== selectedGroupID.value) return
  const ok =
    c.type === 'groups'
      ? await setGroupsHidden(c.ids, c.hidden)
      : await setModelsHidden(c.names, c.hidden)
  if (ok) {
    confirmation.value = null
    selectedGroups.value = []
    selectedModels.value = []
  }
}
</script>
