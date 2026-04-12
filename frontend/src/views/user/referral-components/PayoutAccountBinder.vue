<template>
  <div class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900">
    <div class="mb-4 flex flex-col gap-2 md:flex-row md:items-start md:justify-between">
      <div>
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('referral.payoutAccounts', '收款账户') }}</h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('referral.payoutAccountsHint', '新增提现账户，修改需要 7 天冷却期。') }}</p>
      </div>
    </div>

    <!-- Payout Account List -->
    <div class="space-y-3">
      <div
        v-for="account in accounts"
        :key="account.id"
        class="group relative overflow-hidden rounded-2xl border border-gray-200 p-4 transition hover:border-gray-300 dark:border-dark-700 dark:hover:border-dark-600"
      >
        <div class="flex items-start justify-between gap-3">
          <div>
            <div class="flex items-center gap-2 font-medium text-gray-900 dark:text-white">
              {{ account.account_name }}
              <span class="rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-600 dark:bg-dark-800 dark:text-gray-300">
                {{ account.method }}
              </span>
              <span v-if="account.is_default" class="rounded bg-primary-50 px-2 py-0.5 text-xs text-primary-600 dark:bg-primary-900/20 dark:text-primary-300">
                {{ t('common.default', '默认') }}
              </span>
            </div>
            <div class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ account.account_no_masked || '-' }}
            </div>
          </div>
          <button
            type="button"
            class="btn btn-secondary btn-sm opacity-0 transition group-hover:opacity-100"
            :disabled="!canEditAccount(account)"
            @click="startEdit(account)"
          >
            {{ t('common.edit', '编辑') }}
          </button>
        </div>
        
        <div class="mt-3 flex items-center justify-between text-xs">
          <div v-if="account.qr_image_url" class="text-primary-600 dark:text-primary-400">
            {{ t('referral.hasQrCode', '已绑定收款码') }}
          </div>
          <div v-else></div>
          <div class="text-gray-500 dark:text-gray-400">
            <template v-if="canEditAccount(account)">
              {{ t('referral.accountEditableNow', '现在可修改') }}
            </template>
            <template v-else>
              <div class="flex items-center gap-1 text-amber-600 dark:text-amber-500">
                <svg viewBox="0 0 20 20" fill="currentColor" class="h-4 w-4">
                  <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm.75-13a.75.75 0 00-1.5 0v5c0 .414.336.75.75.75h4a.75.75 0 000-1.5h-3.25V5z" clip-rule="evenodd" />
                </svg>
                {{ t('referral.accountNextEditableAt', { time: formatDate(nextEditableAt(account)) }) }}
              </div>
            </template>
          </div>
        </div>
      </div>

      <!-- Add New Account Button -->
      <button
        type="button"
        class="flex w-full items-center justify-center gap-2 rounded-2xl border border-dashed border-gray-300 py-4 text-sm text-gray-500 transition hover:border-primary-500 hover:text-primary-600 dark:border-dark-600 dark:text-gray-400 dark:hover:border-primary-400 dark:hover:text-primary-400"
        @click="startCreate"
      >
        <svg viewBox="0 0 20 20" fill="currentColor" class="h-5 w-5">
          <path d="M10.75 4.75a.75.75 0 00-1.5 0v4.5h-4.5a.75.75 0 000 1.5h4.5v4.5a.75.75 0 001.5 0v-4.5h4.5a.75.75 0 000-1.5h-4.5v-4.5z" />
        </svg>
        {{ t('referral.addNewAccount', '添加收款账户') }}
      </button>
    </div>

    <!-- Edit/Create Drawer -->
    <BaseDrawer
      :show="drawerOpen"
      :title="editingAccount ? t('referral.updateAccount', '编辑收款账户') : t('referral.addNewAccount', '添加收款账户')"
      size="md"
      @close="closeDrawer"
    >
      <div v-if="!canEditAccountWrapper" class="mb-4 rounded-xl bg-amber-50 p-4 text-sm text-amber-800 dark:bg-amber-900/20 dark:text-amber-200">
        {{ t('referral.accountCoolingDown', '账户冷却中，必须等待冷却时间过后方可修改。') }}
        {{ t('referral.accountNextEditableAt', { time: formatDate(nextEditableAt(editingAccount!)) }) }}
      </div>

      <form class="space-y-6" @submit.prevent="submitForm">
        <!-- Method Switcher -->
        <div>
          <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('referral.payoutMethod', '收款方式') }}
          </label>
          <div class="grid grid-cols-3 gap-2">
            <button
              v-for="method in enabledMethods"
              :key="method"
              type="button"
              class="rounded-xl border py-2 text-center text-sm transition"
              :class="form.method === method ? 'border-primary-500 bg-primary-50 text-primary-700 dark:border-primary-400 dark:bg-primary-900/20 dark:text-primary-300' : 'border-gray-200 bg-white text-gray-700 hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-300 dark:hover:bg-dark-800'"
              @click="form.method = method"
              :disabled="!canEditAccountWrapper && editingAccount !== null"
            >
              {{ t(`referral.methods.${method}`, method.toUpperCase()) }}
            </button>
          </div>
        </div>

        <div class="space-y-4 rounded-2xl border border-gray-100 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800">
          <div>
            <label class="mb-1 block text-sm text-gray-600 dark:text-gray-400">{{ t('referral.accountName', '收款人名称') }}</label>
            <input v-model="form.account_name" class="input bg-white dark:bg-dark-900" :disabled="!canEditAccountWrapper && editingAccount !== null" required />
          </div>

          <!-- Alipay -->
          <template v-if="form.method === 'alipay'">
            <div>
              <label class="mb-1 block text-sm text-gray-600 dark:text-gray-400">{{ t('referral.alipayAccount', '支付宝账号') }}</label>
              <input v-model="form.account_no" class="input bg-white dark:bg-dark-900" :disabled="!canEditAccountWrapper && editingAccount !== null" required />
            </div>
            <div>
              <label class="mb-1 block text-sm text-gray-600 dark:text-gray-400">{{ t('referral.qrCode', '收款二维码 (可选)') }}</label>
              <!-- QR Image Upload Drop Zone -->
              <div
                class="relative mt-1 rounded-xl border-2 border-dashed p-4 text-center transition"
                :class="[
                  qrDragOver ? 'border-primary-500 bg-primary-50 dark:border-primary-400 dark:bg-primary-900/20' : 'border-gray-300 bg-white hover:border-gray-400 dark:border-dark-600 dark:bg-dark-900 dark:hover:border-dark-500',
                  (!canEditAccountWrapper && editingAccount !== null) ? 'pointer-events-none opacity-50' : 'cursor-pointer'
                ]"
                @dragover.prevent="qrDragOver = true"
                @dragleave.prevent="qrDragOver = false"
                @drop.prevent="handleQrDrop"
                @click="triggerQrFileInput"
              >
                <input
                  ref="qrFileInput"
                  type="file"
                  accept="image/png,image/jpeg,image/gif"
                  class="hidden"
                  @change="handleQrFileSelect"
                />
                <div v-if="form.qr_image_url && isDataUrl(form.qr_image_url)" class="flex flex-col items-center gap-2">
                  <img :src="form.qr_image_url" alt="QR Code" class="h-32 w-32 rounded-lg border border-gray-200 object-contain dark:border-dark-700" />
                  <button
                    type="button"
                    class="text-xs text-red-500 hover:text-red-600 dark:text-red-400 dark:hover:text-red-300"
                    @click.stop="form.qr_image_url = ''"
                  >
                    {{ t('common.remove', '移除') }}
                  </button>
                </div>
                <div v-else class="flex flex-col items-center gap-1 py-2">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" class="h-8 w-8 text-gray-400" stroke-width="1.5"><path d="M12 16V4m0 0l-4 4m4-4l4 4" stroke-linecap="round" stroke-linejoin="round"/><path d="M2 17l.621 2.485A2 2 0 004.561 21h14.878a2 2 0 001.94-1.515L22 17" stroke-linecap="round" stroke-linejoin="round"/></svg>
                  <span class="text-sm text-gray-500 dark:text-gray-400">{{ t('referral.dropQrHint', '拖拽图片到此处或点击选择') }}</span>
                  <span class="text-xs text-gray-400 dark:text-gray-500">PNG / JPG / GIF</span>
                </div>
              </div>
              <!-- Preview for URL-based images -->
              <div v-if="form.qr_image_url && !isDataUrl(form.qr_image_url)" class="mt-2 flex items-center gap-2">
                <img :src="form.qr_image_url" alt="QR Code" class="h-16 w-16 rounded-lg border border-gray-200 object-contain dark:border-dark-700" />
              </div>
              <!-- Manual URL input -->
              <input v-model="form.qr_image_url" class="input mt-2 bg-white dark:bg-dark-900" :placeholder="t('referral.qrUrlPlaceholder', '或粘贴图片链接 https://')" :disabled="!canEditAccountWrapper && editingAccount !== null" />
            </div>
          </template>

          <!-- WeChat -->
          <template v-else-if="form.method === 'wechat'">
            <div>
              <label class="mb-1 block text-sm text-gray-600 dark:text-gray-400">{{ t('referral.wechatAccount', '微信号 / 手机号') }}</label>
              <input v-model="form.account_no" class="input bg-white dark:bg-dark-900" :disabled="!canEditAccountWrapper && editingAccount !== null" required />
            </div>
            <div>
              <label class="mb-1 block text-sm text-gray-600 dark:text-gray-400">{{ t('referral.wechatQrUrl', '微信收款码 (可选)') }}</label>
              <!-- QR Image Upload Drop Zone -->
              <div
                class="relative mt-1 rounded-xl border-2 border-dashed p-4 text-center transition"
                :class="[
                  qrDragOver ? 'border-primary-500 bg-primary-50 dark:border-primary-400 dark:bg-primary-900/20' : 'border-gray-300 bg-white hover:border-gray-400 dark:border-dark-600 dark:bg-dark-900 dark:hover:border-dark-500',
                  (!canEditAccountWrapper && editingAccount !== null) ? 'pointer-events-none opacity-50' : 'cursor-pointer'
                ]"
                @dragover.prevent="qrDragOver = true"
                @dragleave.prevent="qrDragOver = false"
                @drop.prevent="handleQrDrop"
                @click="triggerQrFileInput"
              >
                <input
                  ref="qrFileInput"
                  type="file"
                  accept="image/png,image/jpeg,image/gif"
                  class="hidden"
                  @change="handleQrFileSelect"
                />
                <div v-if="form.qr_image_url && isDataUrl(form.qr_image_url)" class="flex flex-col items-center gap-2">
                  <img :src="form.qr_image_url" alt="QR Code" class="h-32 w-32 rounded-lg border border-gray-200 object-contain dark:border-dark-700" />
                  <button
                    type="button"
                    class="text-xs text-red-500 hover:text-red-600 dark:text-red-400 dark:hover:text-red-300"
                    @click.stop="form.qr_image_url = ''"
                  >
                    {{ t('common.remove', '移除') }}
                  </button>
                </div>
                <div v-else class="flex flex-col items-center gap-1 py-2">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" class="h-8 w-8 text-gray-400" stroke-width="1.5"><path d="M12 16V4m0 0l-4 4m4-4l4 4" stroke-linecap="round" stroke-linejoin="round"/><path d="M2 17l.621 2.485A2 2 0 004.561 21h14.878a2 2 0 001.94-1.515L22 17" stroke-linecap="round" stroke-linejoin="round"/></svg>
                  <span class="text-sm text-gray-500 dark:text-gray-400">{{ t('referral.dropQrHint', '拖拽图片到此处或点击选择') }}</span>
                  <span class="text-xs text-gray-400 dark:text-gray-500">PNG / JPG / GIF</span>
                </div>
              </div>
              <!-- Preview for URL-based images -->
              <div v-if="form.qr_image_url && !isDataUrl(form.qr_image_url)" class="mt-2 flex items-center gap-2">
                <img :src="form.qr_image_url" alt="QR Code" class="h-16 w-16 rounded-lg border border-gray-200 object-contain dark:border-dark-700" />
              </div>
              <!-- Manual URL input -->
              <input v-model="form.qr_image_url" class="input mt-2 bg-white dark:bg-dark-900" :placeholder="t('referral.qrUrlPlaceholder', '或粘贴图片链接 https://')" :disabled="!canEditAccountWrapper && editingAccount !== null" />
            </div>
          </template>

          <!-- Bank -->
          <template v-else>
            <div>
              <label class="mb-1 block text-sm text-gray-600 dark:text-gray-400">{{ t('referral.bankName', '开户银行') }}</label>
              <input v-model="form.bank_name" class="input bg-white dark:bg-dark-900" :disabled="!canEditAccountWrapper && editingAccount !== null" required />
            </div>
            <div>
              <label class="mb-1 block text-sm text-gray-600 dark:text-gray-400">{{ t('referral.bankCardNo', '银行卡号') }}</label>
              <input v-model="form.account_no" class="input bg-white dark:bg-dark-900" :disabled="!canEditAccountWrapper && editingAccount !== null" required />
            </div>
          </template>
        </div>

        <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
          <input v-model="form.is_default" type="checkbox" :disabled="!canEditAccountWrapper && editingAccount !== null" />
          {{ t('referral.makeDefaultAccount', '设为默认收款方式') }}
        </label>
      </form>

      <template #footer>
        <div class="flex gap-3">
          <button type="button" class="btn btn-secondary flex-1" @click="closeDrawer">{{ t('common.cancel', '取消') }}</button>
          <button type="button" class="btn btn-primary flex-1" :disabled="saving || (!canEditAccountWrapper && editingAccount !== null)" @click="submitForm">
            {{ saving ? t('common.saving', '保存中') : t('common.save', '保存') }}
          </button>
        </div>
      </template>
    </BaseDrawer>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDrawer from '@/components/common/BaseDrawer.vue'
import referralAPI from '@/api/referral'
import { useAppStore } from '@/stores'
import type { CommissionPayoutAccount } from '@/types'

const { t } = useI18n()
const appStore = useAppStore()

const props = defineProps<{
  accounts: CommissionPayoutAccount[]
  enabledMethods: string[]
}>()

const emit = defineEmits<{
  (e: 'refresh'): void
}>()

const drawerOpen = ref(false)
const saving = ref(false)
const editingAccount = ref<CommissionPayoutAccount | null>(null)

// QR image drag-and-drop
const qrDragOver = ref(false)
const qrFileInput = ref<HTMLInputElement | null>(null)

function isDataUrl(url: string): boolean {
  return url.startsWith('data:')
}

function triggerQrFileInput() {
  qrFileInput.value?.click()
}

function handleQrDrop(event: DragEvent) {
  qrDragOver.value = false
  const file = event.dataTransfer?.files?.[0]
  if (file) processQrFile(file)
}

function handleQrFileSelect(event: Event) {
  const target = event.target as HTMLInputElement
  const file = target.files?.[0]
  if (file) processQrFile(file)
  // Reset input so the same file can be selected again
  target.value = ''
}

function processQrFile(file: File) {
  const validTypes = ['image/png', 'image/jpeg', 'image/gif']
  if (!validTypes.includes(file.type)) {
    appStore.showError(t('referral.invalidImageType', '请选择 PNG、JPG 或 GIF 格式的图片'))
    return
  }
  // Limit to 2MB
  if (file.size > 2 * 1024 * 1024) {
    appStore.showError(t('referral.imageTooLarge', '图片大小不能超过 2MB'))
    return
  }
  const reader = new FileReader()
  reader.onload = (e) => {
    form.qr_image_url = (e.target?.result as string) || ''
  }
  reader.readAsDataURL(file)
}

const form = reactive({
  method: 'alipay',
  account_name: '',
  account_no: '',
  bank_name: '',
  qr_image_url: '',
  is_default: true
})

const canEditAccountWrapper = computed(() => {
  if (!editingAccount.value) return true
  return canEditAccount(editingAccount.value)
})

function nextEditableAt(account: CommissionPayoutAccount) {
  return new Date(new Date(account.updated_at).getTime() + 7 * 24 * 60 * 60 * 1000)
}

function canEditAccount(account: CommissionPayoutAccount) {
  return nextEditableAt(account).getTime() <= Date.now()
}

function formatDate(date: Date) {
  return date.toLocaleString()
}

function startCreate() {
  editingAccount.value = null
  form.method = props.enabledMethods.length ? props.enabledMethods[0] : 'alipay'
  form.account_name = ''
  form.account_no = ''
  form.bank_name = ''
  form.qr_image_url = ''
  form.is_default = props.accounts.length === 0 // true if it's the first account
  drawerOpen.value = true
}

function startEdit(account: CommissionPayoutAccount) {
  editingAccount.value = account
  form.method = account.method
  form.account_name = account.account_name
  form.account_no = ''
  form.bank_name = account.bank_name || ''
  form.qr_image_url = account.qr_image_url || ''
  form.is_default = account.is_default
  drawerOpen.value = true
}

function closeDrawer() {
  drawerOpen.value = false
}

async function submitForm() {
  saving.value = true
  try {
    const payload = {
      method: form.method,
      account_name: form.account_name,
      account_no: form.account_no,
      bank_name: form.bank_name,
      qr_image_url: form.qr_image_url,
      is_default: form.is_default
    }
    
    if (editingAccount.value) {
      await referralAPI.updatePayoutAccount(editingAccount.value.id, payload)
      appStore.showSuccess(t('referral.accountUpdated', '收款账户已更新'))
    } else {
      await referralAPI.createPayoutAccount(payload)
      appStore.showSuccess(t('referral.accountSaved', '收款账户已保存'))
    }
    closeDrawer()
    emit('refresh')
  } catch (error) {
    appStore.showError((error as Error).message || t('common.operationFailed', '操作失败'))
  } finally {
    saving.value = false
  }
}
</script>
