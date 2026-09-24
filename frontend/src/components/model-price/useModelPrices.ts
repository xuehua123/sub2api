import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import api, {
  type ModelPriceResponse,
  type UpdateModelPriceCustomPriceRequest,
} from '@/api/modelPrices'
import { adminAPI } from '@/api/admin'
import { useAppStore, useAuthStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'

export function useModelPrices() {
  const app = useAppStore(),
    auth = useAuthStore()
  const isAdmin = computed(() => auth.isAdmin)
  const response = ref<ModelPriceResponse | null>(null)
  const selectedGroupID = ref<number | undefined>()
  const loading = ref(false),
    busy = ref(false),
    error = ref(''),
    lastAction = ref('')
  const includeCatalog = ref(false),
    showHiddenGroups = ref(false),
    showHiddenModels = ref(false)
  let controller: AbortController | undefined,
    version = 0,
    disposed = false
  const groups = computed(() => response.value?.groups || [])
  const selectedGroup = computed(() =>
    groups.value.find((g) => g.id === selectedGroupID.value),
  )
  const models = computed(() =>
    response.value && response.value.selected_group_id === selectedGroupID.value
      ? response.value.models
      : [],
  )
  async function loadPrices() {
    controller?.abort()
    controller = new AbortController()
    const request = ++version
    const groupID = selectedGroupID.value
    loading.value = true
    error.value = ''
    try {
      const result = await api.getModelPrices({
        group_id: groupID,
        include_catalog: isAdmin.value && includeCatalog.value,
        show_hidden_groups: isAdmin.value && showHiddenGroups.value,
        show_hidden_models: isAdmin.value && showHiddenModels.value,
        signal: controller.signal,
      })
      if (disposed || request !== version) return
      response.value = result
      if (groupID && result.selected_group_id !== groupID) {
        error.value = '该分组已不可访问，请重新选择分组'
        return
      }
    } catch (e) {
      if (disposed || request !== version || controller.signal.aborted) return
      error.value = extractApiErrorMessage(e, '加载价格失败，请重试')
    } finally {
      if (request === version) loading.value = false
    }
  }
  async function mutate(
    action: string,
    work: () => Promise<unknown>,
  ): Promise<boolean> {
    if (!isAdmin.value || busy.value) return false
    busy.value = true
    try {
      await work()
      lastAction.value = action + ' · ' + new Date().toLocaleTimeString('zh-CN')
      app.showSuccess(action)
      await loadPrices()
      return true
    } catch (e) {
      app.showError(extractApiErrorMessage(e, action + '失败'))
      return false
    } finally {
      busy.value = false
    }
  }
  function setGroupsHidden(ids: number[], hidden: boolean) {
    const current = new Set(response.value?.hidden_group_ids || [])
    ids.forEach((id) => (hidden ? current.add(id) : current.delete(id)))
    return mutate(hidden ? '已隐藏所选分组' : '已恢复所选分组', async () => {
      const result=await api.updateHiddenGroups([...current])
      if(hidden && !showHiddenGroups.value && selectedGroupID.value && ids.includes(selectedGroupID.value)) selectedGroupID.value=undefined
      return result
    })
  }
  function setModelsHidden(names: string[], hidden: boolean) {
    const id = selectedGroupID.value
    if (!id) return Promise.resolve(false)
    return mutate(hidden ? '已隐藏所选模型' : '已恢复所选模型', () =>
      api.updateHiddenModels(id, names, hidden),
    )
  }
  function savePrice(body: UpdateModelPriceCustomPriceRequest) {
    return mutate(body.clear ? '已清除展示价覆盖' : '已保存展示价覆盖', () =>
      api.updateCustomPrice(body),
    )
  }
  function syncCatalog() {
    return mutate('目录已同步', () => api.syncCatalog())
  }
  function saveSettings(rate: number, quota: number) {
    if (
      !Number.isFinite(rate) ||
      rate <= 0 ||
      !Number.isFinite(quota) ||
      quota <= 0
    ) {
      app.showError('换算参数必须是大于 0 的有限数')
      return Promise.resolve(false)
    }
    return mutate('展示换算参数已保存', () =>
      adminAPI.settings.updateSettings({
        model_price_usd_cny_rate: rate,
        model_price_cny_per_quota_usd: quota,
      }),
    )
  }
  async function copy(text: string) {
    try {
      await navigator.clipboard.writeText(text)
      app.showSuccess('已复制')
    } catch {
      app.showError('复制失败，请重试')
    }
  }
  watch(
    [selectedGroupID, includeCatalog, showHiddenGroups, showHiddenModels],
    loadPrices,
  )
  onMounted(loadPrices)
  onUnmounted(() => {
    disposed = true
    version++
    controller?.abort()
  })
  return {
    isAdmin,
    response,
    selectedGroupID,
    selectedGroup,
    groups,
    models,
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
  }
}
