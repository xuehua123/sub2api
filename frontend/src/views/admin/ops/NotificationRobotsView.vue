<template>
  <AppLayout>
    <div class="space-y-5 pb-10">
      <header class="flex flex-col gap-4 border-b border-gray-200 pb-4 dark:border-dark-700 lg:flex-row lg:items-center lg:justify-between">
        <div>
          <h1 class="text-xl font-semibold text-gray-900 dark:text-white">告警规则与通知</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">设置何时告警、触发阈值以及通知去向</p>
        </div>
        <div class="flex items-center gap-2">
          <button v-if="hasUnsavedChanges" data-testid="discard-settings" type="button" class="btn btn-secondary h-9" :disabled="loading" @click="loadAll(true)">
            <Icon name="x" size="sm" />
            <span>放弃修改</span>
          </button>
          <button v-else type="button" class="btn btn-secondary h-9" :disabled="loading" @click="loadAll()">
            <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
            <span>刷新</span>
          </button>
          <button type="button" class="btn btn-primary h-9" :disabled="saving || loading || !hasUnsavedChanges" @click="saveAll">
            <Icon name="check" size="sm" />
            <span>{{ saving ? '保存中' : '保存配置' }}</span>
          </button>
        </div>
      </header>

      <div v-if="errorMessage" class="border-l-4 border-red-500 bg-red-50 px-4 py-3 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-200">
        {{ errorMessage }}
      </div>

      <section class="grid grid-cols-2 divide-x divide-y divide-gray-200 border-y border-gray-200 bg-white lg:grid-cols-4 dark:divide-dark-700 dark:border-dark-700 dark:bg-dark-800">
        <div v-for="card in summaryCards" :key="card.label" class="p-3">
          <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ card.label }}</div>
          <div class="mt-2 text-2xl font-semibold tabular-nums" :class="card.className">{{ card.value }}</div>
        </div>
      </section>

      <nav class="inline-flex w-fit border border-gray-200 bg-gray-50 p-1 dark:border-dark-700 dark:bg-dark-900" aria-label="告警配置分类">
        <button v-for="tab in tabs" :key="tab.key" type="button" class="px-4 py-2 text-sm font-medium" :class="activeTab === tab.key ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white' : 'text-gray-500 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white'" @click="activeTab = tab.key">
          {{ tab.label }}
        </button>
      </nav>

      <section v-if="activeTab === 'health'" class="border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
        <div class="flex flex-col gap-4 border-b border-gray-200 p-4 dark:border-dark-700 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <div class="flex items-center gap-3">
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">账号健康通知总开关</h2>
              <label class="inline-flex items-center gap-2 text-sm font-medium text-gray-700 dark:text-gray-200">
                <input v-model="healthSettings.enabled" type="checkbox" class="checkbox" />
                {{ healthSettings.enabled ? '已启用' : '已关闭' }}
              </label>
            </div>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">关闭后仍保留健康监控，但不会发送任何账号健康通知。</p>
          </div>
          <div class="grid grid-cols-2 gap-3 sm:w-[420px]">
            <label class="space-y-1">
              <span class="text-xs font-medium text-gray-500 dark:text-gray-400">通知账号范围</span>
              <select v-model="healthSettings.mode" class="input h-9">
                <option value="smart">智能：异常看已开启，恢复按规则</option>
                <option value="opened_only">仅已开启账号</option>
                <option value="all">全部账号</option>
              </select>
            </label>
            <label class="space-y-1">
              <span class="text-xs font-medium text-gray-500 dark:text-gray-400">每小时通知上限</span>
              <input v-model.number="healthSettings.rate_limit_per_hour" type="number" min="0" max="1000" class="input h-9" />
            </label>
          </div>
        </div>

        <div class="divide-y divide-gray-200 dark:divide-dark-700">
          <article class="grid gap-4 p-4 xl:grid-cols-[minmax(260px,1fr)_minmax(520px,2fr)] xl:items-start">
            <div>
              <label class="flex items-center gap-2 text-sm font-semibold text-gray-900 dark:text-white">
                <input v-model="healthSettings.burst.enabled" data-testid="health-burst-enabled" type="checkbox" class="checkbox" />
                短时错误率告警
              </label>
              <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ burstRuleText }}</p>
            </div>
            <div class="grid grid-cols-2 gap-3 sm:grid-cols-5">
              <label class="space-y-1"><span class="text-xs text-gray-500">观察窗口（分）</span><input v-model.number="healthSettings.burst.window_minutes" data-testid="health-burst-window" type="number" min="1" max="1440" class="input h-9" /></label>
              <label class="space-y-1"><span class="text-xs text-gray-500">最少请求数</span><input v-model.number="healthSettings.burst.min_requests" type="number" min="1" class="input h-9" /></label>
              <label class="space-y-1"><span class="text-xs text-gray-500">错误率达到 %</span><input v-model.number="healthSettings.burst.error_rate_percent" type="number" min="0" max="100" class="input h-9" /></label>
              <label class="space-y-1"><span class="text-xs text-gray-500">上游错误达到 %</span><input v-model.number="healthSettings.burst.upstream_error_rate_percent" type="number" min="0" max="100" class="input h-9" /></label>
              <label class="space-y-1"><span class="text-xs text-gray-500">重复提醒间隔（分）</span><input v-model.number="healthSettings.burst.cooldown_minutes" type="number" min="0" class="input h-9" /></label>
              <label class="col-span-2 inline-flex items-center gap-2 text-xs text-gray-600 dark:text-gray-300 sm:col-span-5"><input v-model="healthSettings.burst.bypass_digest" type="checkbox" class="checkbox" />命中后立即通知，不等待汇总</label>
            </div>
          </article>

          <article class="grid gap-4 p-4 xl:grid-cols-[minmax(260px,1fr)_minmax(520px,2fr)] xl:items-start">
            <div>
              <label class="flex items-center gap-2 text-sm font-semibold text-gray-900 dark:text-white">
                <input v-model="healthSettings.degrade.enabled" data-testid="health-degrade-enabled" type="checkbox" class="checkbox" />
                持续成功率下降告警
              </label>
              <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ degradeRuleText }}</p>
            </div>
            <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
              <label class="space-y-1"><span class="text-xs text-gray-500">观察窗口（分）</span><input v-model.number="healthSettings.degrade.window_minutes" type="number" min="1" class="input h-9" /></label>
              <label class="space-y-1"><span class="text-xs text-gray-500">最少请求数</span><input v-model.number="healthSettings.degrade.min_requests" type="number" min="1" class="input h-9" /></label>
              <label class="space-y-1"><span class="text-xs text-gray-500">成功率低于 %</span><input v-model.number="healthSettings.degrade.success_rate_min_percent" type="number" min="0" max="100" class="input h-9" /></label>
              <label class="space-y-1"><span class="text-xs text-gray-500">错误率达到 %</span><input v-model.number="healthSettings.degrade.error_rate_percent" type="number" min="0" max="100" class="input h-9" /></label>
              <label class="space-y-1"><span class="text-xs text-gray-500">上游错误达到 %</span><input v-model.number="healthSettings.degrade.upstream_error_rate_percent" type="number" min="0" max="100" class="input h-9" /></label>
              <label class="space-y-1"><span class="text-xs text-gray-500">重复提醒间隔（分）</span><input v-model.number="healthSettings.degrade.cooldown_minutes" type="number" min="0" class="input h-9" /></label>
            </div>
          </article>

          <article class="grid gap-4 p-4 xl:grid-cols-[minmax(260px,1fr)_minmax(520px,2fr)] xl:items-start">
            <div>
              <label class="flex items-center gap-2 text-sm font-semibold text-gray-900 dark:text-white">
                <input v-model="healthSettings.recovery.enabled" data-testid="health-recovery-enabled" type="checkbox" class="checkbox" />
                恢复通知
              </label>
              <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ recoveryRuleText }}</p>
            </div>
            <div class="grid grid-cols-2 gap-3 sm:grid-cols-4">
              <label class="space-y-1"><span class="text-xs text-gray-500">观察窗口（分）</span><input v-model.number="healthSettings.recovery.window_minutes" type="number" min="1" class="input h-9" /></label>
              <label class="space-y-1"><span class="text-xs text-gray-500">最少请求数</span><input v-model.number="healthSettings.recovery.min_requests" type="number" min="1" class="input h-9" /></label>
              <label class="space-y-1"><span class="text-xs text-gray-500">成功率达到 %</span><input v-model.number="healthSettings.recovery.success_rate_min_percent" type="number" min="0" max="100" class="input h-9" /></label>
              <label class="space-y-1"><span class="text-xs text-gray-500">重复提醒间隔（分）</span><input v-model.number="healthSettings.recovery.cooldown_minutes" type="number" min="0" class="input h-9" /></label>
              <label class="col-span-2 inline-flex items-center gap-2 text-xs text-gray-600 dark:text-gray-300"><input v-model="healthSettings.recovery.notify_opened_accounts" type="checkbox" class="checkbox" />已开启账号恢复后通知</label>
              <label class="col-span-2 inline-flex items-center gap-2 text-xs text-gray-600 dark:text-gray-300"><input v-model="healthSettings.recovery.notify_closed_accounts" type="checkbox" class="checkbox" />已关闭账号达到可恢复条件时通知</label>
            </div>
          </article>

          <article class="grid gap-4 p-4 xl:grid-cols-[minmax(260px,1fr)_minmax(520px,2fr)] xl:items-start">
            <div>
              <label class="flex items-center gap-2 text-sm font-semibold text-gray-900 dark:text-white">
                <input v-model="healthSettings.probe.enabled" type="checkbox" class="checkbox" />
                账号主动探测
              </label>
              <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">这是检测规则，不会单独发通知；检测结果由上方三类规则判断。</p>
            </div>
            <div class="grid grid-cols-2 gap-3 sm:grid-cols-5">
              <label class="space-y-1"><span class="text-xs text-gray-500">探测间隔（分）</span><input v-model.number="healthSettings.probe.interval_minutes" type="number" min="1" max="1440" class="input h-9" /></label>
              <label class="space-y-1"><span class="text-xs text-gray-500">每轮账号数</span><input v-model.number="healthSettings.probe.max_per_run" type="number" min="1" max="20" class="input h-9" /></label>
              <label class="space-y-1"><span class="text-xs text-gray-500">单次超时（秒）</span><input v-model.number="healthSettings.probe.timeout_seconds" type="number" min="1" class="input h-9" /></label>
              <label class="space-y-1"><span class="text-xs text-gray-500">探测模式</span><select v-model="healthSettings.probe.mode" class="input h-9"><option value="default">默认连接测试</option><option value="compact">OpenAI compact</option></select></label>
              <label class="space-y-1"><span class="text-xs text-gray-500">探测模型</span><input v-model.trim="healthSettings.probe.model_id" type="text" class="input h-9" /></label>
            </div>
          </article>
        </div>
      </section>

      <section v-if="activeTab === 'balance'" class="border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
        <div class="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <div class="flex items-center gap-3">
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">共享上游低余额告警</h2>
              <label class="inline-flex items-center gap-2 text-sm font-medium text-gray-700 dark:text-gray-200"><input v-model="balanceSettings.enabled" type="checkbox" class="checkbox" />{{ balanceSettings.enabled ? '已启用' : '已关闭' }}</label>
            </div>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">只读取共享上游连接的钱包快照，不再探测单个账号余额。</p>
          </div>
          <div class="grid grid-cols-2 gap-3 sm:w-[420px]">
            <label class="space-y-1"><span class="text-xs font-medium text-gray-500">默认低余额阈值（USD）</span><input v-model.number="balanceSettings.default_threshold_usd" data-testid="balance-default-threshold" type="number" min="0" step="0.01" class="input h-9" /></label>
            <label class="space-y-1"><span class="text-xs font-medium text-gray-500">单连接每小时通知上限</span><input v-model.number="balanceSettings.rate_limit_per_hour" type="number" min="0" max="1000" class="input h-9" /></label>
          </div>
        </div>
      </section>

      <section v-if="activeTab === 'channels'" class="grid grid-cols-1 border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800 xl:grid-cols-2">
        <div class="p-4 xl:border-r xl:border-gray-200 xl:dark:border-dark-700">
          <div class="flex items-center justify-between gap-3"><div><h2 class="text-base font-semibold text-gray-900 dark:text-white">账号健康通知</h2><p class="mt-1 text-xs text-gray-500 dark:text-gray-400">错误率、成功率与恢复消息使用此机器人。</p></div><button type="button" class="btn btn-secondary btn-sm" :disabled="testingHealthRobot" @click="testHealthRobot"><Icon name="mail" size="xs" /><span>{{ testingHealthRobot ? '发送中' : '测试机器人' }}</span></button></div>
          <label class="mt-4 inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200"><input v-model="healthSettings.notification.enterprise_wechat_enabled" type="checkbox" class="checkbox" />启用企业微信通知</label>
          <input v-model.trim="healthSettings.notification.enterprise_wechat_webhook_url" type="password" class="input mt-3" autocomplete="off" placeholder="https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=..." />
          <label class="mt-3 inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200"><input v-model="healthSettings.notification.mention_all_on_immediate" type="checkbox" class="checkbox" />短时错误率告警时 @所有人</label>
        </div>
        <div class="border-t border-gray-200 p-4 dark:border-dark-700 xl:border-t-0">
          <div class="flex items-center justify-between gap-3"><div><h2 class="text-base font-semibold text-gray-900 dark:text-white">低余额通知</h2><p class="mt-1 text-xs text-gray-500 dark:text-gray-400">共享上游连接低于阈值时使用此机器人。</p></div><button type="button" class="btn btn-secondary btn-sm" :disabled="testingBalanceRobot" @click="testBalanceRobot"><Icon name="mail" size="xs" /><span>{{ testingBalanceRobot ? '发送中' : '测试机器人' }}</span></button></div>
          <label class="mt-4 inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200"><input v-model="balanceSettings.notification.enterprise_wechat_enabled" type="checkbox" class="checkbox" />启用企业微信通知</label>
          <input v-model.trim="balanceSettings.notification.enterprise_wechat_webhook_url" type="password" class="input mt-3" autocomplete="off" placeholder="https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=..." />
          <label class="mt-3 inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200"><input v-model="balanceSettings.notification.mention_all_on_low_balance" type="checkbox" class="checkbox" />低余额告警时 @所有人</label>
        </div>
      </section>

      <section v-if="activeTab === 'balance'" class="border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
        <div class="border-b border-gray-200 p-4 dark:border-dark-700">
          <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
            <div>
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">上游连接余额</h2>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">阈值留空时使用全局默认值；未绑定账号的连接不会触发告警</p>
            </div>
            <div class="flex flex-wrap items-center gap-2">
              <input v-model.trim="filters.q" type="search" class="input h-9 w-56" placeholder="搜索连接或错误" @keyup.enter="applyFilters" />
              <select v-model="filters.status" class="input h-9 w-36" @change="applyFilters">
                <option value="">全部状态</option>
                <option value="ready">正常</option>
                <option value="degraded">降级</option>
                <option value="auth_error">认证失败</option>
                <option value="needs_input">需要补充信息</option>
                <option value="disabled">已停用</option>
              </select>
              <label class="inline-flex h-9 items-center gap-2 border border-gray-200 px-3 text-xs text-gray-600 dark:border-dark-700 dark:text-gray-300">
                <input v-model="filters.only_low" data-testid="balance-only-low" type="checkbox" class="checkbox" @change="applyFilters" />
                仅低余额
              </label>
              <label class="inline-flex h-9 items-center gap-2 border border-gray-200 px-3 text-xs text-gray-600 dark:border-dark-700 dark:text-gray-300">
                <input v-model="filters.only_failed" type="checkbox" class="checkbox" @change="applyFilters" />
                仅异常
              </label>
            </div>
          </div>
        </div>

        <div class="overflow-x-auto">
          <table class="w-full min-w-[1050px] table-fixed divide-y divide-gray-200 text-sm dark:divide-dark-700">
            <colgroup>
              <col class="w-[25%]" />
              <col class="w-[16%]" />
              <col class="w-[16%]" />
              <col class="w-[16%]" />
              <col class="w-[15%]" />
              <col class="w-[12%]" />
            </colgroup>
            <thead class="bg-gray-50 text-xs text-gray-500 dark:bg-dark-900/40 dark:text-gray-400">
              <tr>
                <th class="px-4 py-2.5 text-left font-semibold">连接</th>
                <th class="px-4 py-2.5 text-left font-semibold">上游余额</th>
                <th class="px-4 py-2.5 text-left font-semibold">同步状态</th>
                <th class="px-4 py-2.5 text-left font-semibold">告警阈值</th>
                <th class="px-4 py-2.5 text-left font-semibold">告警</th>
                <th class="px-4 py-2.5 text-right font-semibold">刷新</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-if="loading && items.length === 0">
                <td colspan="6" class="px-4 py-10 text-center text-sm text-gray-500">加载中...</td>
              </tr>
              <tr v-else-if="items.length === 0">
                <td colspan="6" class="px-4 py-10 text-center text-sm text-gray-500">没有符合条件的共享上游连接</td>
              </tr>
              <tr v-for="item in items" :key="item.connection_id" :class="rowClass(item)">
                <td class="px-4 py-3 align-middle">
                  <div class="truncate font-medium text-gray-900 dark:text-white" :title="item.name">{{ item.name }}</div>
                  <div class="mt-1 flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
                    <span>{{ providerLabel(item.provider) }}</span>
                    <span>绑定 {{ item.binding_count }} 个账号</span>
                  </div>
                </td>
                <td class="px-4 py-3 align-middle">
                  <div class="font-semibold tabular-nums" :class="walletClass(item)">{{ formatWallet(item) }}</div>
                  <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ item.wallet_source || '尚未同步' }}</div>
                </td>
                <td class="px-4 py-3 align-middle">
                  <span class="inline-flex px-2 py-0.5 text-xs font-semibold" :class="statusClass(item.status)">{{ statusLabel(item.status) }}</span>
                  <div class="mt-1 truncate text-xs" :class="item.last_error ? 'text-red-500' : 'text-gray-500 dark:text-gray-400'" :title="item.last_error">
                    {{ syncHint(item) }}
                  </div>
                </td>
                <td class="px-4 py-3 align-middle">
                  <div class="flex items-center gap-2">
                    <span class="text-gray-500">$</span>
                    <input
                      :value="item.alert.uses_default_threshold ? '' : item.alert.threshold_usd"
                      type="number"
                      min="0"
                      step="0.01"
                      class="input h-8 w-24 text-sm"
                      :disabled="updatingIds.has(item.connection_id)"
                      :placeholder="String(balanceSettings.default_threshold_usd)"
                      @change="onThresholdChange(item, $event)"
                    />
                  </div>
                  <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ item.alert.uses_default_threshold ? '使用全局默认' : '连接专用' }}</div>
                </td>
                <td class="px-4 py-3 align-middle">
                  <label class="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
                    <input
                      :checked="item.alert.enabled"
                      type="checkbox"
                      class="checkbox"
                      :disabled="updatingIds.has(item.connection_id)"
                      @change="onAlertEnabledChange(item, $event)"
                    />
                    <span>{{ alertLabel(item) }}</span>
                  </label>
                  <div v-if="!item.alert.eligible" class="mt-1 text-xs text-gray-500">未绑定或同步已停用</div>
                </td>
                <td class="px-4 py-3 text-right align-middle">
                  <button
                    type="button"
                    class="inline-flex h-8 w-8 items-center justify-center text-blue-600 hover:bg-blue-50 disabled:opacity-50 dark:text-blue-400 dark:hover:bg-blue-950/30"
                    :disabled="probingIds.has(item.connection_id)"
                    title="刷新连接余额"
                    aria-label="刷新连接余额"
                    @click="probeItem(item)"
                  >
                    <Icon name="refresh" size="sm" :class="{ 'animate-spin': probingIds.has(item.connection_id) }" />
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <div class="flex flex-col gap-3 border-t border-gray-200 px-4 py-3 dark:border-dark-700 sm:flex-row sm:items-center sm:justify-between">
          <div class="text-sm text-gray-500 dark:text-gray-400">共 {{ response?.total ?? 0 }} 个连接</div>
          <div class="flex items-center gap-2">
            <button type="button" class="btn btn-secondary btn-sm" :disabled="filters.page <= 1 || loading" @click="changePage(filters.page - 1)">上一页</button>
            <span class="text-sm text-gray-600 dark:text-gray-300">第 {{ filters.page }} 页</span>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="isLastPage || loading" @click="changePage(filters.page + 1)">下一页</button>
          </div>
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import {
  getAccountHealth,
  getUpstreamConnectionBalanceMonitor,
  probeUpstreamConnectionBalance,
  testAccountHealthEnterpriseWeChat,
  testUpstreamConnectionBalanceEnterpriseWeChat,
  updateAccountHealthSettings,
  updateUpstreamConnectionBalanceAlert,
  updateUpstreamConnectionBalanceSettings,
  type OpsAccountHealthSettings,
  type OpsAccountHealthResponse,
  type OpsUpstreamConnectionBalanceItem,
  type OpsUpstreamConnectionBalanceListResponse,
  type OpsUpstreamConnectionBalanceSettings
} from '@/api/admin/ops'

const appStore = useAppStore()
const fullLoading = ref(false)
const balanceLoading = ref(false)
const loading = computed(() => fullLoading.value || balanceLoading.value)
const saving = ref(false)
const testingBalanceRobot = ref(false)
const testingHealthRobot = ref(false)
const errorMessage = ref('')
type AlertTab = 'health' | 'balance' | 'channels'
const activeTab = ref<AlertTab>('health')
const tabs: Array<{ key: AlertTab; label: string }> = [
  { key: 'health', label: '账号健康规则' },
  { key: 'balance', label: '上游余额规则' },
  { key: 'channels', label: '通知渠道' }
]
const response = ref<OpsUpstreamConnectionBalanceListResponse | null>(null)
const healthResponse = ref<OpsAccountHealthResponse | null>(null)
const settingsBaseline = ref('')
const fullLoadRequestSeq = ref(0)
const balanceRequestSeq = ref(0)
const balanceSettingsLoaded = ref(false)
const probingIds = ref(new Set<number>())
const updatingIds = ref(new Set<number>())
const balanceSettings = reactive<OpsUpstreamConnectionBalanceSettings>(defaultBalanceSettings())
const healthSettings = reactive<OpsAccountHealthSettings>(defaultHealthSettings())
const filters = reactive({ page: 1, page_size: 20, q: '', status: '', only_low: false, only_failed: false })

const items = computed(() => response.value?.items ?? [])
const isLastPage = computed(() => filters.page * filters.page_size >= (response.value?.total ?? 0))
const hasUnsavedChanges = computed(() => settingsBaseline.value !== '' && settingsBaseline.value !== settingsSnapshot())
const summaryCards = computed(() => {
  const healthItems = healthResponse.value?.items ?? []
  const immediate = healthItems.filter(item => item.recommendation?.notify_mode === 'immediate').length
  const digest = healthItems.filter(item => item.recommendation?.notify_mode === 'digest').length
  const recovery = healthItems.filter(item => item.recommendation?.recovery_ready).length
  const lowBalance = response.value?.summary.low_balance_connections ?? 0
  return [
    { label: '立即告警', value: immediate, className: immediate ? 'text-red-600 dark:text-red-300' : 'text-gray-900 dark:text-white' },
    { label: '汇总告警', value: digest, className: digest ? 'text-amber-600 dark:text-amber-300' : 'text-gray-900 dark:text-white' },
    { label: '可恢复', value: recovery, className: recovery ? 'text-emerald-600 dark:text-emerald-300' : 'text-gray-900 dark:text-white' },
    { label: '低余额连接', value: lowBalance, className: lowBalance ? 'text-amber-600 dark:text-amber-300' : 'text-gray-900 dark:text-white' }
  ]
})
const burstRuleText = computed(() => healthSettings.burst.enabled
  ? `${healthSettings.burst.window_minutes} 分钟内至少 ${healthSettings.burst.min_requests} 次请求，错误率达到 ${healthSettings.burst.error_rate_percent}% 或上游错误率达到 ${healthSettings.burst.upstream_error_rate_percent}% 时触发。`
  : '已关闭：短时间大量错误不会发送通知。')
const degradeRuleText = computed(() => healthSettings.degrade.enabled
  ? `${healthSettings.degrade.window_minutes} 分钟窗口至少 ${healthSettings.degrade.min_requests} 次请求，成功率低于 ${healthSettings.degrade.success_rate_min_percent}% 或错误率达到阈值时触发。`
  : '已关闭：持续成功率下降不会发送通知。')
const recoveryRuleText = computed(() => healthSettings.recovery.enabled
  ? `${healthSettings.recovery.window_minutes} 分钟内至少 ${healthSettings.recovery.min_requests} 次请求且成功率达到 ${healthSettings.recovery.success_rate_min_percent}% 时触发。`
  : '已关闭：账号恢复或达到可开启条件时不会发送通知。')

onMounted(() => void loadAll())

async function loadAll(_discardChanges = false) {
  const requestSeq = ++fullLoadRequestSeq.value
  const balanceSeq = ++balanceRequestSeq.value
  fullLoading.value = true
  errorMessage.value = ''
  try {
    const [balance, health] = await Promise.all([
      getUpstreamConnectionBalanceMonitor({ ...filters }),
      getAccountHealth({ recent_limit: 1 })
    ])
    if (requestSeq !== fullLoadRequestSeq.value) return
    healthResponse.value = health
    applyHealthSettings(health.settings)
    applyBalanceSettings(balance.settings)
    balanceSettingsLoaded.value = true
    if (balanceSeq === balanceRequestSeq.value) response.value = balance
    settingsBaseline.value = settingsSnapshot()
  } catch (error) {
    if (requestSeq !== fullLoadRequestSeq.value) return
    errorMessage.value = errorMessageOf(error, '加载通知配置失败')
  } finally {
    if (requestSeq === fullLoadRequestSeq.value) fullLoading.value = false
  }
}

async function loadBalance() {
  const requestSeq = ++balanceRequestSeq.value
  balanceLoading.value = true
  try {
    const balance = await getUpstreamConnectionBalanceMonitor({ ...filters })
    if (requestSeq !== balanceRequestSeq.value) return
    response.value = balance
    if (!balanceSettingsLoaded.value) {
      applyBalanceSettings(balance.settings)
      balanceSettingsLoaded.value = true
    }
  } catch (error) {
    if (requestSeq !== balanceRequestSeq.value) return
    appStore.showError(errorMessageOf(error, '加载上游连接余额失败'))
  } finally {
    if (requestSeq === balanceRequestSeq.value) balanceLoading.value = false
  }
}

async function saveAll() {
  const validationError = validateSettings()
  if (validationError) {
    appStore.showError(validationError)
    return
  }
  saving.value = true
  try {
    const [balanceResult, healthResult] = await Promise.allSettled([
      updateUpstreamConnectionBalanceSettings(toPlain(balanceSettings)),
      updateAccountHealthSettings(toPlain(healthSettings))
    ])
    if (balanceResult.status === 'fulfilled') applyBalanceSettings(balanceResult.value)
    if (healthResult.status === 'fulfilled') applyHealthSettings(healthResult.value)
    if (balanceResult.status === 'fulfilled' && healthResult.status === 'fulfilled') {
      settingsBaseline.value = settingsSnapshot()
      appStore.showSuccess('通知配置已保存')
      return
    }

    const failures: string[] = []
    if (balanceResult.status === 'rejected') failures.push(`余额规则：${errorMessageOf(balanceResult.reason, '保存失败')}`)
    if (healthResult.status === 'rejected') failures.push(`健康规则：${errorMessageOf(healthResult.reason, '保存失败')}`)
    await loadAll(true)
    appStore.showError(`部分配置未保存。${failures.join('；')}`)
  } finally {
    saving.value = false
  }
}

async function testBalanceRobot() {
  testingBalanceRobot.value = true
  try {
    await testUpstreamConnectionBalanceEnterpriseWeChat(toPlain(balanceSettings))
    appStore.showSuccess('余额测试消息已发送')
  } catch (error) {
    appStore.showError(errorMessageOf(error, '余额测试发送失败'))
  } finally {
    testingBalanceRobot.value = false
  }
}

async function testHealthRobot() {
  testingHealthRobot.value = true
  try {
    await testAccountHealthEnterpriseWeChat(toPlain(healthSettings))
    appStore.showSuccess('健康测试消息已发送')
  } catch (error) {
    appStore.showError(errorMessageOf(error, '健康测试发送失败'))
  } finally {
    testingHealthRobot.value = false
  }
}

async function patchAlert(item: OpsUpstreamConnectionBalanceItem, payload: { enabled?: boolean; threshold_usd?: number; use_default_threshold?: boolean }) {
  if (updatingIds.value.has(item.connection_id)) return
  updatingIds.value = new Set(updatingIds.value).add(item.connection_id)
  try {
    item.alert = await updateUpstreamConnectionBalanceAlert(item.connection_id, payload)
    await loadBalance()
    appStore.showSuccess('连接告警配置已更新')
  } catch (error) {
    appStore.showError(errorMessageOf(error, '更新连接告警失败'))
    await loadBalance()
  } finally {
    const next = new Set(updatingIds.value)
    next.delete(item.connection_id)
    updatingIds.value = next
  }
}

function onAlertEnabledChange(item: OpsUpstreamConnectionBalanceItem, event: Event) {
  const target = event.target as HTMLInputElement | null
  if (target) void patchAlert(item, { enabled: target.checked })
}

function onThresholdChange(item: OpsUpstreamConnectionBalanceItem, event: Event) {
  const value = String((event.target as HTMLInputElement | null)?.value ?? '').trim()
  if (!value) {
    void patchAlert(item, { use_default_threshold: true })
    return
  }
  const threshold = Number(value)
  if (!Number.isFinite(threshold) || threshold < 0) {
    appStore.showError('阈值必须是非负数字')
    return
  }
  void patchAlert(item, { threshold_usd: threshold })
}

async function probeItem(item: OpsUpstreamConnectionBalanceItem) {
  if (probingIds.value.has(item.connection_id)) return
  probingIds.value = new Set(probingIds.value).add(item.connection_id)
  try {
    const refreshed = await probeUpstreamConnectionBalance(item.connection_id)
    const index = response.value?.items.findIndex((row) => row.connection_id === item.connection_id) ?? -1
    if (response.value && index >= 0) response.value.items[index] = refreshed
    await loadBalance()
    appStore.showSuccess('共享连接余额已刷新')
  } catch (error) {
    appStore.showError(errorMessageOf(error, '刷新共享连接失败'))
  } finally {
    const next = new Set(probingIds.value)
    next.delete(item.connection_id)
    probingIds.value = next
  }
}

function applyFilters() {
  filters.page = 1
  void loadBalance()
}

function changePage(page: number) {
  filters.page = Math.max(1, page)
  void loadBalance()
}

function applyBalanceSettings(settings: OpsUpstreamConnectionBalanceSettings) {
  Object.assign(balanceSettings, defaultBalanceSettings(), settings)
  balanceSettings.notification = { ...defaultBalanceSettings().notification, ...(settings.notification ?? {}) }
}

function applyHealthSettings(settings: OpsAccountHealthSettings) {
  const defaults = defaultHealthSettings()
  Object.assign(healthSettings, defaults, settings)
  healthSettings.burst = { ...defaults.burst, ...(settings.burst ?? {}) }
  healthSettings.degrade = { ...defaults.degrade, ...(settings.degrade ?? {}) }
  healthSettings.recovery = { ...defaults.recovery, ...(settings.recovery ?? {}) }
  healthSettings.probe = { ...defaults.probe, ...(settings.probe ?? {}) }
  healthSettings.notification = { ...defaults.notification, ...(settings.notification ?? {}) }
}

function formatWallet(item: OpsUpstreamConnectionBalanceItem) {
  if (item.wallet_unlimited) return '额度未设上限'
  if (item.wallet_amount != null) return formatCurrency(item.wallet_amount, item.wallet_currency)
  if (item.wallet_usd != null) return formatCurrency(item.wallet_usd, 'USD')
  return '未知'
}

function formatCurrency(amount: number, currency = 'USD') {
  const value = Number(amount).toFixed(2)
  const unit = currency.trim().toUpperCase() || 'USD'
  if (unit === 'USD') return `$${value}`
  if (unit === 'CNY') return `¥${value}`
  return `${unit} ${value}`
}

function walletClass(item: OpsUpstreamConnectionBalanceItem) {
  if (item.alert.low) return 'text-amber-600 dark:text-amber-300'
  if (item.last_error) return 'text-red-600 dark:text-red-300'
  if (item.wallet_unlimited || item.wallet_amount != null || item.wallet_usd != null) return 'text-emerald-600 dark:text-emerald-300'
  return 'text-gray-500 dark:text-gray-400'
}

function rowClass(item: OpsUpstreamConnectionBalanceItem) {
  if (item.alert.low) return 'bg-amber-50/50 dark:bg-amber-950/10'
  if (item.last_error) return 'bg-red-50/40 dark:bg-red-950/10'
  return ''
}

function alertLabel(item: OpsUpstreamConnectionBalanceItem) {
  if (!balanceSettings.enabled) return '全局暂停'
  if (!item.alert.enabled) return '已关闭'
  if (item.alert.low) return '低余额'
  return '监控中'
}

function syncHint(item: OpsUpstreamConnectionBalanceItem) {
  if (item.last_error) return item.last_error
  if (!item.wallet_observed_at) return '等待首次同步'
  const date = new Date(item.wallet_observed_at)
  if (Number.isNaN(date.getTime())) return '同步时间未知'
  return item.alert.snapshot_fresh ? date.toLocaleString() : `${date.toLocaleString()}（已过期）`
}

function statusLabel(status: string) {
  const labels: Record<string, string> = {
    ready: '正常', degraded: '降级', pending: '待同步', auth_error: '认证失败',
    needs_input: '需要信息', disabled: '已停用'
  }
  return labels[status] ?? (status || '未知')
}

function statusClass(status: string) {
  if (status === 'ready') return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
  if (status === 'auth_error' || status === 'needs_input') return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
  if (status === 'degraded') return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
  return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
}

function providerLabel(provider: string) {
  return provider === 'auto' ? '自动识别' : provider
}

function validateSettings() {
  if (!Number.isFinite(balanceSettings.default_threshold_usd) || balanceSettings.default_threshold_usd < 0) return '默认余额阈值必须是非负数字。'
  if (!Number.isFinite(healthSettings.rate_limit_per_hour) || healthSettings.rate_limit_per_hour < 0 || healthSettings.rate_limit_per_hour > 1000) return '账号健康每小时通知上限必须在 0 到 1000 之间。'
  if (!Number.isFinite(balanceSettings.rate_limit_per_hour) || balanceSettings.rate_limit_per_hour < 0 || balanceSettings.rate_limit_per_hour > 1000) return '单连接低余额每小时通知上限必须在 0 到 1000 之间。'
  const percentages = [
    healthSettings.burst.error_rate_percent,
    healthSettings.burst.upstream_error_rate_percent,
    healthSettings.degrade.success_rate_min_percent,
    healthSettings.degrade.error_rate_percent,
    healthSettings.degrade.upstream_error_rate_percent,
    healthSettings.recovery.success_rate_min_percent
  ]
  if (percentages.some(value => !Number.isFinite(value) || value < 0 || value > 100)) return '成功率和错误率阈值必须在 0% 到 100% 之间。'
  const windows = [healthSettings.burst.window_minutes, healthSettings.degrade.window_minutes, healthSettings.recovery.window_minutes]
  if (windows.some(value => !Number.isFinite(value) || value < 1 || value > 1440)) return '账号健康观察窗口必须在 1 到 1440 分钟之间。'
  const cooldowns = [healthSettings.burst.cooldown_minutes, healthSettings.degrade.cooldown_minutes, healthSettings.recovery.cooldown_minutes]
  if (cooldowns.some(value => !Number.isFinite(value) || value < 0)) return '账号健康重复提醒间隔必须是非负数字。'
  const requestMinimums = [healthSettings.burst.min_requests, healthSettings.degrade.min_requests, healthSettings.recovery.min_requests]
  if (requestMinimums.some(value => !Number.isFinite(value) || value < 1)) return '账号健康最少请求数必须大于 0。'
  const probeNumbers = [healthSettings.probe.interval_minutes, healthSettings.probe.max_per_run, healthSettings.probe.timeout_seconds]
  if (probeNumbers.some(value => !Number.isFinite(value) || value < 1)) return '主动探测间隔、每轮账号数和超时必须大于 0。'
  if (balanceSettings.notification.enterprise_wechat_enabled && !hasWebhook(balanceSettings.notification.enterprise_wechat_webhook_url)) {
    return '已开启共享连接余额提醒，请先填写企业微信 Webhook。'
  }
  if (healthSettings.notification.enterprise_wechat_enabled && !hasWebhook(healthSettings.notification.enterprise_wechat_webhook_url)) {
    return '已开启账号健康提醒，请先填写企业微信 Webhook。'
  }
  return ''
}

function hasWebhook(value?: string) {
  return String(value ?? '').trim() !== ''
}

function errorMessageOf(error: unknown, fallback: string) {
  if (typeof error !== 'object' || error === null) return fallback
  const candidate = error as { response?: { data?: { message?: string; error?: string } }; message?: string }
  return candidate.response?.data?.message || candidate.response?.data?.error || candidate.message || fallback
}

function toPlain<T>(value: T): T {
  return JSON.parse(JSON.stringify(value))
}

function settingsSnapshot(): string {
  return JSON.stringify({
    balance: toPlain(balanceSettings),
    health: toPlain(healthSettings)
  })
}

function defaultBalanceSettings(): OpsUpstreamConnectionBalanceSettings {
  return {
    enabled: true,
    default_threshold_usd: 10,
    rate_limit_per_hour: 12,
    notification: {
      enterprise_wechat_enabled: false,
      enterprise_wechat_webhook_url: '',
      mention_all_on_low_balance: false
    }
  }
}

function defaultHealthSettings(): OpsAccountHealthSettings {
  return {
    enabled: true,
    mode: 'smart',
    burst: { enabled: true, window_minutes: 1, min_requests: 10, error_rate_percent: 50, upstream_error_rate_percent: 25, cooldown_minutes: 1, bypass_digest: true },
    degrade: { enabled: true, window_minutes: 10, min_requests: 20, success_rate_min_percent: 90, error_rate_percent: 20, upstream_error_rate_percent: 10, cooldown_minutes: 10 },
    recovery: { enabled: true, window_minutes: 30, min_requests: 10, success_rate_min_percent: 98, notify_opened_accounts: true, notify_closed_accounts: true, cooldown_minutes: 30 },
    probe: { enabled: true, interval_minutes: 30, max_per_run: 2, timeout_seconds: 20, model_id: 'gpt-5.4-mini', mode: 'default', prompt: '' },
    notification: { enterprise_wechat_enabled: false, enterprise_wechat_webhook_url: '', mention_all_on_immediate: false },
    rate_limit_per_hour: 12
  }
}
</script>
