<template>
  <AppLayout>
    <div class="mx-auto w-full max-w-[1400px] px-4 py-6 text-slate-900 sm:px-6 lg:px-8 dark:text-slate-100">
      <div v-if="loading" class="flex items-center justify-center py-32">
        <div class="relative flex h-12 w-12 items-center justify-center">
          <div class="absolute inset-0 animate-ping rounded-full bg-primary-500/30"></div>
          <div class="h-8 w-8 animate-spin rounded-full border-4 border-primary-500 border-t-transparent"></div>
        </div>
      </div>
      
      <template v-else>
        <!-- Payment in progress Phase -->
        <template v-if="paymentPhase === 'paying'">
          <div class="mx-auto max-w-2xl rounded-2xl border border-slate-200/70 bg-white/90 p-6 shadow-2xl shadow-slate-200/70 backdrop-blur-xl dark:border-white/5 dark:bg-[#090f1d]/60 dark:shadow-black/30">
            <PaymentStatusPanel
              :order-id="paymentState.orderId"
              :qr-code="paymentState.qrCode"
              :expires-at="paymentState.expiresAt"
              :payment-type="paymentState.paymentType"
              :pay-url="paymentState.payUrl"
              :order-type="paymentState.orderType"
              :currency="paymentState.currency || selectedCurrency"
              @done="onPaymentDone"
              @success="onPaymentSuccess"
              @settled="onPaymentSettled"
            />
          </div>
        </template>

        <!-- Selection Phase -->
        <template v-else>
          <!-- Page Header & Tab Switcher (Show only in Selection Step) -->
          <div v-if="!selectedPlan" class="text-center mb-10 space-y-4">
            <template v-if="activeTab === 'subscription'">
              <h1 class="text-3xl font-extrabold tracking-tight text-slate-950 sm:text-4xl md:text-5xl dark:text-white">
                选择更稳的 <span class="text-transparent bg-clip-text bg-gradient-to-r from-primary-400 to-cyan-400">AI API</span> 额度
              </h1>
              <p class="mx-auto max-w-2xl text-sm leading-relaxed text-slate-600 sm:text-base dark:text-slate-400">
                全面兼容 GPT-5 / Claude Code / OpenAI 系列模型及生态工具，稳定接口调度，原倍率计费，成本公开透明。
              </p>
            </template>
            <template v-else>
              <h1 class="text-3xl font-extrabold tracking-tight text-slate-950 sm:text-4xl md:text-5xl dark:text-white">
                账户余额充值
              </h1>
              <p class="mx-auto max-w-2xl text-sm leading-relaxed text-slate-600 sm:text-base dark:text-slate-400">
                安全快捷，即时入账。充值余额可用于所有分组的按量计费抵扣。
              </p>
            </template>
            
            <!-- Tab Switcher -->
            <div v-if="tabs.length > 1" class="inline-flex rounded-xl border border-slate-200/80 bg-white/[0.85] p-1 shadow-lg shadow-slate-200/70 backdrop-blur-md dark:border-white/5 dark:bg-slate-900/60 dark:shadow-black/20">
              <button
                v-for="tab in tabs"
                :key="tab.key"
                class="h-9 px-6 rounded-lg text-xs font-bold transition-all duration-200"
                :class="activeTab === tab.key ? 'bg-primary-500 text-white shadow-md shadow-primary-500/20' : 'text-slate-500 hover:text-slate-900 dark:text-slate-400 dark:hover:text-slate-200'"
                @click="setActiveTab(tab.key)"
              >
                {{ tab.label }}
              </button>
            </div>

            <!-- Chips -->
            <div class="flex flex-wrap justify-center gap-1.5 pt-2">
              <span
                v-for="chip in heroCapabilityChips"
                :key="chip"
                class="rounded-full border border-slate-200/80 bg-white/75 px-2.5 py-0.5 text-[11px] font-medium text-slate-500 shadow-sm dark:border-white/5 dark:bg-white/[0.02] dark:text-slate-400"
              >
                {{ chip }}
              </span>
            </div>
          </div>

          <!-- 1. Subscription Tab -->
          <template v-if="activeTab === 'subscription'">
            
            <!-- Step 1: Package Selection (selectedPlan is null) -->
            <template v-if="!selectedPlan">
              <div v-if="checkout.plans.length === 0" class="card py-24 text-center glass-panel">
                <Icon name="gift" size="xl" class="mx-auto mb-4 text-slate-500" />
                <p class="text-slate-500 dark:text-slate-400">{{ t('payment.noPlans') }}</p>
              </div>
              
              <template v-else>
                <!-- Grid of Plan Cards -->
                <div class="grid grid-cols-1 gap-6 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-5 max-w-[1600px] mx-auto">
                  <div
                    v-for="plan in subscriptionPlans"
                    :key="plan.id"
                    :class="[
                      'group relative flex flex-col rounded-2xl border p-6 text-left transition-all duration-300',
                      isRecommendedPlan(plan)
                        ? 'recommended-card border-primary-300/70 bg-gradient-to-b from-white via-cyan-50/80 to-white shadow-xl shadow-cyan-100/80 hover:-translate-y-1 hover:scale-[1.01] hover:border-primary-400/80 hover:shadow-2xl hover:shadow-cyan-400/20 dark:border-primary-500/25 dark:from-[#0d2229]/50 dark:via-[#0a1120]/40 dark:to-[#070c16]/30 dark:shadow-[0_0_40px_rgba(20,184,166,0.08)] dark:hover:shadow-cyan-500/10'
                        : 'border-slate-200/80 bg-white/[0.92] shadow-xl shadow-slate-200/70 hover:-translate-y-1 hover:border-primary-200 hover:bg-white hover:shadow-2xl hover:shadow-slate-300/40 dark:border-white/5 dark:bg-[#090f1d]/40 dark:shadow-lg dark:hover:border-white/10 dark:hover:bg-[#0c1528]/50 dark:hover:shadow-black/30'
                    ]"
                  >
                    <!-- Recommended Badge -->
                    <span
                      v-if="isRecommendedPlan(plan)"
                      class="absolute -top-3 left-6 rounded-full bg-gradient-to-r from-primary-500 to-cyan-500 px-3 py-0.5 text-[10px] font-black uppercase tracking-wider text-slate-950 shadow-md shadow-primary-500/20"
                    >
                      推荐 · 最佳性价比
                    </span>

                    <!-- Card Header -->
                    <div class="space-y-1">
                      <span class="text-[11px] font-bold uppercase tracking-wide text-slate-500 dark:text-slate-400">
                        {{ planAudience(plan) }}
                      </span>
                      <h3 class="text-lg font-bold text-slate-950 transition-colors group-hover:text-primary-600 dark:text-white dark:group-hover:text-primary-300">
                        {{ plan.name }}
                      </h3>
                    </div>

                    <!-- Price -->
                    <div class="mt-4 flex items-baseline gap-1">
                      <span class="text-3xl font-extrabold tracking-tight text-slate-950 dark:text-white">
                        {{ formatPlanPrice(plan) }}
                      </span>
                      <span class="text-xs text-slate-500 dark:text-slate-400">/ {{ validitySuffixForPlan(plan) }}</span>
                    </div>

                    <!-- Original Price / Discount -->
                    <div class="mt-1 flex items-center gap-2 min-h-[20px]">
                      <span v-if="plan.original_price && plan.original_price > plan.price" class="text-xs text-slate-400 line-through dark:text-slate-500">
                        {{ formatSelectedPaymentAmount(plan.original_price) }}
                      </span>
                      <span v-if="discountText(plan)" class="rounded-md border border-amber-300/40 bg-amber-100 px-1.5 py-0.5 text-[10px] font-bold text-amber-700 dark:border-amber-400/20 dark:bg-amber-400/10 dark:text-amber-300">
                        {{ discountText(plan) }}
                      </span>
                    </div>

                    <!-- Metrics Grid -->
                    <div class="mt-5 space-y-2.5 border-t border-slate-200/80 pt-5 text-xs dark:border-white/5">
                      <div class="flex items-center justify-between">
                        <span class="text-slate-500 dark:text-slate-400">月度额度</span>
                        <span class="font-bold text-slate-950 dark:text-white">{{ planQuotaText(plan) }}</span>
                      </div>
                      <div class="flex items-center justify-between">
                        <span class="text-slate-500 dark:text-slate-400">每刀成本</span>
                        <span class="font-bold text-slate-950 dark:text-white">{{ unitCostText(plan) }}</span>
                      </div>
                    </div>

                    <!-- Covered Groups -->
                    <div class="mt-4 border-t border-slate-200/80 pt-4 dark:border-white/5">
                      <div class="flex items-center justify-between text-xs">
                        <span class="text-slate-500 dark:text-slate-400">覆盖分组</span>
                        <span class="rounded border border-primary-200 bg-primary-50 px-1.5 py-0.5 font-bold text-primary-700 dark:border-primary-500/20 dark:bg-primary-500/10 dark:text-primary-300">
                          {{ planGroupCountText(plan) }}
                        </span>
                      </div>
                      <div class="mt-2.5 space-y-1.5">
                        <div
                          v-for="group in getPlanView(plan.id).cardGroups"
                          :key="group.key"
                          class="flex flex-col gap-1 rounded-lg border border-slate-200/80 bg-slate-50 px-2.5 py-2 text-[10px] dark:border-white/5 dark:bg-slate-900/60 transition-all hover:bg-slate-100/50 dark:hover:bg-slate-800/40"
                        >
                          <div class="flex items-center justify-between gap-2">
                            <div class="flex items-center gap-1.5 min-w-0">
                              <span
                                v-if="group.platform"
                                :class="['shrink-0 rounded-full px-1 py-0.5 text-[8px] font-bold uppercase tracking-wider', platformBadgeLightClass(group.platform)]"
                              >
                                {{ platformLabel(group.platform) }}
                              </span>
                              <span class="truncate font-semibold text-slate-700 dark:text-slate-300">{{ group.name }}</span>
                            </div>
                            <span class="shrink-0 rounded-md border border-emerald-200 bg-emerald-50 px-1.5 py-0.5 font-extrabold text-emerald-700 dark:border-emerald-500/20 dark:bg-emerald-500/10 dark:text-emerald-300">
                              {{ group.multiplierText }}
                            </span>
                          </div>
                          <div v-if="group.actualCostText !== null" class="flex items-center justify-between text-[9px] text-slate-400 dark:text-slate-500 font-medium leading-none">
                            <span>{{ GROUP_LABELS.actualCost }}</span>
                            <span class="font-bold text-emerald-600 dark:text-emerald-400">
                              {{ group.actualCostText }}
                            </span>
                          </div>
                        </div>
                        <button
                          v-if="getPlanView(plan.id).cardOverflowCount > 0"
                          type="button"
                          class="w-full rounded-lg border border-primary-200 bg-primary-50 px-2.5 py-1.5 text-left text-[10px] font-bold text-primary-700 transition hover:border-primary-300 hover:bg-primary-100 dark:border-primary-500/20 dark:bg-primary-500/10 dark:text-primary-300 dark:hover:border-primary-400 dark:hover:bg-primary-500/15"
                          @click.stop="openPlanGroupsModal(plan)"
                        >
                          +{{ getPlanView(plan.id).cardOverflowCount }} 个分组 · 查看全部
                        </button>
                      </div>
                    </div>

                    <!-- Description / Compact Summary -->
                    <div class="plan-description-clamp mt-5 text-[10px] italic leading-[1.55] text-slate-500 dark:text-slate-400">
                      {{ planCardSummaryText(plan) }}
                    </div>

                    <!-- Spacer -->
                    <div class="flex-1 min-h-[20px]"></div>

                    <!-- Card Button -->
                    <button
                      type="button"
                      :class="[
                        'w-full flex items-center justify-center gap-1.5 rounded-xl py-2.5 text-xs font-bold transition-all duration-200 active:scale-[0.98]',
                        isRecommendedPlan(plan)
                          ? 'bg-gradient-to-r from-primary-500 to-cyan-500 text-slate-950 shadow-lg shadow-primary-500/25 hover:from-primary-400 hover:to-cyan-400'
                          : 'border border-primary-200 bg-primary-50 text-primary-700 hover:border-primary-300 hover:bg-primary-100 dark:border-primary-500/20 dark:bg-white/[0.04] dark:text-primary-300 dark:hover:border-primary-400 dark:hover:bg-primary-500/10 dark:hover:text-white'
                      ]"
                      @click="selectPlan(plan)"
                    >
                      <span>选择此套餐</span>
                      <Icon name="arrowRight" size="xs" />
                    </button>
                  </div>
                </div>

                <!-- Accordion Comparison Toggle -->
                <div class="mt-12 flex flex-col items-center justify-center max-w-5xl mx-auto w-full space-y-4">
                  <button
                    type="button"
                    class="flex items-center gap-2 rounded-xl border border-slate-200/80 bg-white/80 px-5 py-2.5 text-sm font-bold text-slate-600 shadow-sm transition-all hover:bg-white hover:text-slate-950 dark:border-white/5 dark:bg-slate-900/40 dark:text-slate-300 dark:hover:bg-slate-800/40 dark:hover:text-white"
                    @click="isComparisonOpen = !isComparisonOpen"
                  >
                    <span>对比套餐详细权益及说明</span>
                    <svg
                      class="h-4 w-4 transition-transform duration-200"
                      :class="isComparisonOpen ? 'rotate-180 text-primary-400' : ''"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                      stroke-width="2.5"
                    >
                      <path stroke-linecap="round" stroke-linejoin="round" d="M19.5 8.25l-7.5 7.5-7.5-7.5" />
                    </svg>
                  </button>
                  
                  <div v-if="isComparisonOpen" class="w-full rounded-2xl border border-slate-200/80 bg-white/[0.88] p-6 shadow-xl shadow-slate-200/70 backdrop-blur-md transition-all duration-300 dark:border-white/5 dark:bg-[#090f1d]/45 dark:shadow-black/20">
                    <!-- Custom table style without heavy borders -->
                    <div class="overflow-x-auto rounded-xl">
                      <table class="w-full min-w-[700px] border-collapse text-left text-xs text-slate-700 dark:text-slate-300">
                        <thead>
                          <tr class="border-b border-slate-200/80 text-[11px] font-bold uppercase tracking-wider text-slate-500 dark:border-white/5 dark:text-slate-400">
                            <th class="pb-3 pr-4">权益项目</th>
                            <th v-for="plan in subscriptionPlans" :key="plan.id" class="pb-3 px-4" :class="{'text-primary-600 dark:text-primary-300 font-extrabold': isRecommendedPlan(plan)}">
                              {{ plan.name }}
                            </th>
                          </tr>
                        </thead>
                        <tbody class="divide-y divide-slate-200/70 dark:divide-white/5">
                          <tr>
                            <td class="py-3 pr-4 font-medium text-slate-500 dark:text-slate-400">月度价格</td>
                            <td v-for="plan in subscriptionPlans" :key="plan.id" class="py-3 px-4 font-bold text-slate-900 dark:text-white">
                              {{ formatPlanPrice(plan) }} / {{ validitySuffixForPlan(plan) }}
                            </td>
                          </tr>
                          <tr>
                            <td class="py-3 pr-4 font-medium text-slate-500 dark:text-slate-400">原价 / 折扣</td>
                            <td v-for="plan in subscriptionPlans" :key="plan.id" class="py-3 px-4">
                              <span v-if="plan.original_price && plan.original_price > plan.price" class="mr-2 text-slate-400 line-through dark:text-slate-500">
                                {{ formatSelectedPaymentAmount(plan.original_price) }}
                              </span>
                              <span v-if="discountText(plan)" class="rounded bg-amber-100 px-1.5 py-0.5 font-bold text-amber-700 dark:bg-amber-400/10 dark:text-amber-300">
                                {{ discountText(plan) }}
                              </span>
                              <span v-else class="text-slate-400 dark:text-slate-500">-</span>
                            </td>
                          </tr>
                          <tr>
                            <td class="py-3 pr-4 font-medium text-slate-500 dark:text-slate-400">月度额度</td>
                            <td v-for="plan in subscriptionPlans" :key="plan.id" class="py-3 px-4 font-bold text-slate-900 dark:text-white">
                              {{ planQuotaText(plan) }}
                            </td>
                          </tr>
                          <tr>
                            <td class="py-3 pr-4 font-medium text-slate-500 dark:text-slate-400">每刀成本</td>
                            <td v-for="plan in subscriptionPlans" :key="plan.id" class="py-3 px-4 font-semibold text-slate-900 dark:text-white">
                              {{ unitCostText(plan) }}
                            </td>
                          </tr>
                          <tr>
                            <td class="py-3 pr-4 font-medium text-slate-500 dark:text-slate-400">适合人群</td>
                            <td v-for="plan in subscriptionPlans" :key="plan.id" class="py-3 px-4 text-slate-600 dark:text-slate-400">
                              {{ planAudience(plan) }}
                            </td>
                          </tr>
                          <tr>
                            <td class="py-3 pr-4 font-medium text-slate-500 dark:text-slate-400">套餐说明</td>
                            <td v-for="plan in subscriptionPlans" :key="plan.id" class="py-3 px-4 text-slate-600 dark:text-slate-400">
                              <p class="max-w-[220px] leading-relaxed">
                                {{ planDescriptionText(plan) || '-' }}
                              </p>
                            </td>
                          </tr>
                          <tr>
                            <td class="py-3 pr-4 font-medium text-slate-500 dark:text-slate-400">包含权益</td>
                            <td v-for="plan in subscriptionPlans" :key="plan.id" class="py-3 px-4">
                              <div class="flex flex-wrap gap-1">
                                <span v-for="feat in compactPlanFeatures(plan)" :key="feat" class="inline-flex items-center gap-1 rounded border border-slate-200/80 bg-slate-50 px-1.5 py-0.5 text-[10px] text-slate-600 dark:border-white/5 dark:bg-white/[0.02] dark:text-slate-300">
                                  <svg class="h-3 w-3 text-primary-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="3">
                                    <path stroke-linecap="round" stroke-linejoin="round" d="M4.5 12.75l6 6 9-13.5" />
                                  </svg>
                                  {{ feat }}
                                </span>
                              </div>
                            </td>
                          </tr>
                          <tr>
                            <td class="py-3 pr-4 font-medium text-slate-500 dark:text-slate-400">包含分组</td>
                            <td v-for="plan in subscriptionPlans" :key="plan.id" class="py-3 px-4">
                              <div class="space-y-1.5">
                                <div
                                  v-for="group in getPlanView(plan.id).compareGroups"
                                  :key="group.key"
                                  class="flex items-center justify-between gap-2 rounded-lg border border-slate-200/80 bg-slate-50 px-2 py-1 dark:border-white/5 dark:bg-slate-900/60"
                                >
                                  <span class="min-w-0 truncate text-[10px] text-slate-700 dark:text-slate-300">{{ group.name }}</span>
                                  <span class="shrink-0 text-[10px] font-black text-emerald-700 dark:text-emerald-300">{{ group.multiplierText }}</span>
                                </div>
                                <button
                                  v-if="getPlanView(plan.id).compareOverflowCount > 0"
                                  type="button"
                                  class="block text-left text-[10px] font-bold text-primary-700 transition hover:text-primary-500 dark:text-primary-300 dark:hover:text-primary-200"
                                  @click="openPlanGroupsModal(plan)"
                                >
                                  +{{ getPlanView(plan.id).compareOverflowCount }} 个分组 · 查看全部
                                </button>
                              </div>
                            </td>
                          </tr>
                        </tbody>
                      </table>
                    </div>
                  </div>
                </div>
              </template>
            </template>

            <!-- Step 2: Payment Confirmation (selectedPlan is not null) -->
            <template v-else>
              <div class="mb-6">
                <button
                  type="button"
                  class="inline-flex items-center gap-1.5 text-sm font-semibold text-slate-500 transition-colors hover:text-slate-950 dark:text-slate-400 dark:hover:text-white"
                  @click="selectedPlan = null"
                >
                  <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M15.75 19.5L8.25 12l7.5-7.5" />
                  </svg>
                  <span>返回选择套餐</span>
                </button>
              </div>

              <div class="grid grid-cols-1 lg:grid-cols-[1fr_420px] gap-8 max-w-6xl mx-auto items-start">
                
                <!-- Left: Selected Package Overview & Group details -->
                <div class="space-y-6">
                  <div class="rounded-2xl border border-slate-200/80 bg-white/90 p-6 shadow-xl shadow-slate-200/70 backdrop-blur-md dark:border-white/5 dark:bg-[#090f1d]/40 dark:shadow-black/20">
                    <h2 class="mb-6 flex items-center gap-2 text-xl font-bold text-slate-950 dark:text-white">
                      <span class="inline-block w-1.5 h-5 bg-primary-500 rounded-full"></span>
                      确认您的订阅套餐
                    </h2>

                    <!-- Plan Summary Header -->
                    <div class="flex flex-wrap items-start justify-between gap-4 border-b border-slate-200/80 pb-6 dark:border-white/5">
                      <div class="space-y-1">
                        <div class="flex items-center gap-2">
                          <h3 class="text-2xl font-extrabold text-slate-950 dark:text-white">{{ selectedPlan.name }}</h3>
                          <span v-if="isRecommendedPlan(selectedPlan)" class="rounded-full border border-primary-200 bg-primary-50 px-2.5 py-0.5 text-[10px] font-bold text-primary-700 dark:border-primary-500/30 dark:bg-primary-500/10 dark:text-primary-300">
                            推荐套餐
                          </span>
                        </div>
                        <p class="text-xs text-slate-500 dark:text-slate-400">{{ selectedPlanSummaryText }}</p>
                        <p v-if="planDescriptionText(selectedPlan)" class="mt-2 max-w-2xl text-xs leading-relaxed text-slate-600 dark:text-slate-300">
                          {{ planDescriptionText(selectedPlan) }}
                        </p>
                      </div>

                      <div class="text-right">
                        <div class="flex items-baseline gap-1 justify-end">
                          <span class="text-3xl font-extrabold text-primary-400">{{ formatPlanPrice(selectedPlan) }}</span>
                          <span class="text-xs text-slate-500 dark:text-slate-400">/ {{ validitySuffixForPlan(selectedPlan) }}</span>
                        </div>
                        <div v-if="selectedPlan.original_price && selectedPlan.original_price > selectedPlan.price" class="text-xs text-slate-400 line-through dark:text-slate-500">
                          原价 {{ formatSelectedSubscriptionPaymentAmount(selectedPlan.original_price) }}
                        </div>
                      </div>
                    </div>

                    <!-- Quota & Cost Grid -->
                    <div class="grid grid-cols-2 gap-4 mt-6">
                      <div class="rounded-xl border border-slate-200/80 bg-slate-50/80 p-4 dark:border-white/5 dark:bg-white/[0.01]">
                        <span class="block text-xs text-slate-500">月度额度</span>
                        <span class="mt-1 block text-lg font-bold text-slate-950 dark:text-white">{{ planQuotaText(selectedPlan) }}</span>
                      </div>
                      <div class="rounded-xl border border-slate-200/80 bg-slate-50/80 p-4 dark:border-white/5 dark:bg-white/[0.01]">
                        <span class="block text-xs text-slate-500">每刀成本</span>
                        <span class="mt-1 block text-lg font-bold text-slate-950 dark:text-white">{{ unitCostText(selectedPlan) }}</span>
                      </div>
                    </div>

                    <!-- Group platform coverage list -->
                    <div class="mt-6 space-y-3">
                      <h4 class="text-xs font-bold uppercase tracking-wider text-slate-500">包含的分组权益与费率</h4>
                      
                      <div class="space-y-2">
                        <div
                          v-for="group in getPlanView(selectedPlan?.id).groupRows"
                          :key="group.key"
                          class="flex items-center justify-between rounded-xl border border-slate-200/80 bg-slate-50/90 p-4 dark:border-white/5 dark:bg-slate-950/40"
                        >
                          <div class="space-y-0.5">
                            <span class="text-sm font-bold text-slate-950 dark:text-white">
                              {{ group.name }}
                            </span>
                            <div class="flex flex-wrap items-center gap-2 text-[10px] font-medium text-slate-500 dark:text-slate-400">
                              <template v-for="(item, idx) in group.metadataLines" :key="idx">
                                <span v-if="idx > 0">·</span>
                                <span :class="{ 'font-bold text-emerald-600 dark:text-emerald-400': idx === group.metadataLines.length - 1 }">
                                  {{ item }}
                                </span>
                              </template>
                            </div>
                          </div>
                          <span class="rounded-md border border-emerald-200 bg-emerald-50 px-2 py-1 text-xs font-bold text-emerald-700 dark:border-emerald-500/25 dark:bg-emerald-500/10 dark:text-emerald-300">
                            倍率 {{ group.multiplierText }}
                          </span>
                        </div>
                      </div>

                      <!-- Price disclaimer / Fluctuation notice -->
                      <div class="mt-4 flex gap-2 rounded-xl border border-amber-300/30 bg-amber-500/5 p-3.5 text-xs text-amber-600/90 dark:border-amber-500/20 dark:text-amber-400/90">
                        <Icon name="infoCircle" size="sm" class="shrink-0 mt-0.5 text-amber-500" />
                        <div class="leading-relaxed">
                          <p class="font-bold">关于倍率后实际单价与费率波动的说明：</p>
                          <p class="mt-0.5 text-[11px] opacity-85">每个分组的“倍率后实际单价”是根据当前套餐的每刀成本乘以该分组的计费倍率估算而得。分组通道费率可能会根据市场和汇率进行调整波动，最终计费以实际调用时系统设定的费率为准。</p>
                        </div>
                      </div>
                    </div>

                  </div>
                </div>

                <!-- Right: Secure Checkout Panel -->
                <aside class="space-y-6 rounded-2xl border border-slate-200/80 bg-white/[0.92] p-6 shadow-xl shadow-slate-200/80 backdrop-blur-md dark:border-white/5 dark:bg-[#090f1d]/60 dark:shadow-black/20 lg:sticky lg:top-24">
                  <div>
                    <h2 class="text-lg font-extrabold text-slate-950 dark:text-white">订单支付详情</h2>
                    <p class="mt-1 text-xs text-slate-500 dark:text-slate-400">请核对订单金额并选择合适的支付通道完成付款</p>
                  </div>

                  <!-- Price details -->
                  <div class="space-y-3 rounded-xl border border-slate-200/80 bg-slate-50/80 p-4 text-xs dark:border-white/5 dark:bg-white/[0.01]">
                    <div class="flex justify-between text-slate-500 dark:text-slate-400">
                      <span>套餐价格</span>
                      <span class="font-semibold text-slate-950 dark:text-white">{{ formatSelectedSubscriptionPaymentAmount(selectedPlan.price) }}</span>
                    </div>
                    <div class="flex justify-between text-slate-500 dark:text-slate-400">
                      <span>通道手续费</span>
                      <span class="font-semibold text-slate-950 dark:text-white">{{ formatSelectedPaymentAmount(subFeeAmount) }}</span>
                    </div>
                    <div class="flex justify-between border-t border-slate-200/80 pt-3 text-sm dark:border-white/5">
                      <span class="font-bold text-slate-700 dark:text-slate-200">应付总额</span>
                      <span class="text-xl font-extrabold text-primary-400">{{ formatSelectedPaymentAmount(subTotalAmount) }}</span>
                    </div>
                  </div>

                  <div v-if="planHasPeakRate(selectedPlan)" class="rounded-xl border border-amber-500/15 bg-amber-500/5 p-3">
                    <span class="text-xs text-amber-700 dark:text-amber-300">{{ t('payment.planCard.peakRate') }}</span>
                    <div class="text-sm font-semibold text-amber-800 dark:text-amber-200">
                      {{ planPeakRateLabel(selectedPlan) }}
                    </div>
                  </div>
                  <div v-if="selectedPlan.daily_limit_usd != null" class="rounded-xl border border-slate-200/80 bg-slate-50/80 p-3 dark:border-white/5 dark:bg-white/[0.01]">
                    <span class="text-xs text-slate-500 dark:text-slate-400">{{ t('payment.planCard.dailyLimit') }}</span>
                    <div class="text-sm font-semibold text-slate-950 dark:text-white">${{ selectedPlan.daily_limit_usd }}</div>
                  </div>

                  <!-- Payment methods selector -->
                  <div class="space-y-2">
                    <p class="text-xs font-bold text-slate-500 dark:text-slate-400">选择支付方式</p>
                    <div v-if="enabledMethods.length >= 1" class="grid grid-cols-2 gap-3">
                      <button
                        v-for="method in subMethodOptions"
                        :key="method.type"
                        type="button"
                        :disabled="!method.available"
                        :class="[
                          'relative flex flex-col items-center justify-center rounded-xl border p-4 text-center transition-all duration-200',
                          !method.available
                            ? 'cursor-not-allowed border-slate-200 bg-slate-50 opacity-40 dark:border-white/5 dark:bg-white/[0.01]'
                            : selectedMethod === method.type
                              ? 'border-primary-400 bg-primary-50 text-slate-950 shadow-[0_0_15px_rgba(20,184,166,0.15)] dark:border-primary-500 dark:bg-primary-500/10 dark:text-white'
                              : 'border-slate-200/80 bg-white text-slate-600 hover:border-primary-200 hover:text-slate-950 dark:border-white/5 dark:bg-slate-900/40 dark:text-slate-300 dark:hover:border-white/10 dark:hover:text-white'
                        ]"
                        @click="method.available && (selectedMethod = method.type)"
                      >
                        <!-- Selector checkmark -->
                        <span v-if="selectedMethod === method.type" class="absolute top-1.5 right-1.5 flex h-3.5 w-3.5 items-center justify-center rounded-full bg-primary-500 text-slate-950">
                          <Icon name="checkCircle" size="xs" />
                        </span>

                        <img :src="methodIconSrc(method.type)" :alt="paymentMethodLabel(method)" class="h-6 w-6 object-contain" />
                        <span class="text-xs font-bold mt-2">{{ paymentMethodLabel(method) }}</span>
                      </button>
                    </div>
                    <div v-else class="rounded-xl border border-amber-500/15 bg-amber-500/5 p-3 text-xs text-amber-300">
                      暂未在系统后台开启任何可用的支付渠道，请联系管理员。
                    </div>
                  </div>

                  <!-- Main Pay Button -->
                  <button
                    type="button"
                    class="w-full flex h-12 items-center justify-center gap-2 rounded-xl text-sm font-extrabold text-slate-950 shadow-lg transition-all duration-200 active:scale-[0.99] disabled:cursor-not-allowed btn-shimmer disabled:bg-slate-800 disabled:text-slate-500 disabled:shadow-none"
                    :disabled="!canSubmitSubscription || submitting"
                    @click="confirmSubscribe"
                  >
                    <span v-if="submitting" class="h-4 w-4 animate-spin rounded-full border-2 border-slate-950 border-t-transparent"></span>
                    <span>{{ subscriptionCtaLabel }}</span>
                    <Icon v-if="!submitting && canSubmitSubscription" name="arrowRight" size="xs" />
                  </button>

                  <!-- Trust info -->
                  <div class="grid grid-cols-2 gap-3 border-t border-slate-200/80 pt-5 text-center text-[10px] text-slate-500 dark:border-white/5 dark:text-slate-400">
                    <span class="flex items-center justify-center gap-1.5 rounded-lg bg-slate-50 py-2 dark:bg-white/[0.01]">
                      <Icon name="shield" size="xs" class="text-primary-400" />
                      安全可信
                    </span>
                    <span class="flex items-center justify-center gap-1.5 rounded-lg bg-slate-50 py-2 dark:bg-white/[0.01]">
                      <Icon name="clock" size="xs" class="text-primary-400" />
                      极速生效
                    </span>
                    <span class="flex items-center justify-center gap-1.5 rounded-lg bg-slate-50 py-2 dark:bg-white/[0.01]">
                      <Icon name="document" size="xs" class="text-primary-400" />
                      支持发票
                    </span>
                    <span class="flex items-center justify-center gap-1.5 rounded-lg bg-slate-50 py-2 dark:bg-white/[0.01]">
                      <Icon name="server" size="xs" class="text-primary-400" />
                      分组调度
                    </span>
                  </div>

                </aside>

              </div>
            </template>

          </template>

          <!-- 2. Recharge Tab -->
          <template v-else-if="activeTab === 'recharge'">
            <div class="grid grid-cols-1 lg:grid-cols-[1fr_420px] gap-8 max-w-6xl mx-auto items-start">
              
              <!-- Left: Recharge Amount Selection -->
              <div class="space-y-6">
                <!-- Account Info -->
                <div class="flex items-center justify-between rounded-2xl border border-slate-200/80 bg-white/90 p-6 shadow-xl shadow-slate-200/70 backdrop-blur-md dark:border-white/5 dark:bg-[#090f1d]/40 dark:shadow-black/20">
                  <div class="space-y-1">
                    <span class="block text-xs text-slate-500">当前充值账户</span>
                    <span class="block text-base font-bold text-slate-950 dark:text-white">{{ user?.username || '' }}</span>
                  </div>
                  <div class="text-right">
                    <span class="block text-xs text-slate-500">当前余额</span>
                    <span class="mt-0.5 block text-2xl font-extrabold text-emerald-600 dark:text-emerald-400">
                      ${{ user?.balance?.toFixed(2) || '0.00' }}
                    </span>
                  </div>
                </div>

                <!-- Preset Amount Selector -->
                <div class="space-y-6 rounded-2xl border border-slate-200/80 bg-white/90 p-6 shadow-xl shadow-slate-200/70 backdrop-blur-md dark:border-white/5 dark:bg-[#090f1d]/40 dark:shadow-black/20">
                  <h2 class="text-lg font-bold text-slate-950 dark:text-white">选择充值金额</h2>
                  
                  <div v-if="enabledMethods.length === 0" class="py-12 text-center text-xs text-slate-500">
                    充值功能暂未开放
                  </div>

                  <template v-else>
                    <AmountInput
                      v-model="amount"
                      :amounts="[10, 20, 50, 100, 200, 500, 1000, 2000, 5000]"
                      :min="globalMinAmount"
                      :max="globalMaxAmount"
                    />
                    <p v-if="amountError" class="text-xs text-amber-400">{{ amountError }}</p>
                  </template>
                </div>
              </div>

              <!-- Right: Checkout Sidebar -->
              <aside class="space-y-6 rounded-2xl border border-slate-200/80 bg-white/[0.92] p-6 shadow-xl shadow-slate-200/80 backdrop-blur-md dark:border-white/5 dark:bg-[#090f1d]/60 dark:shadow-black/20 lg:sticky lg:top-24">
                <div>
                  <h2 class="text-lg font-extrabold text-slate-950 dark:text-white">余额充值详情</h2>
                  <p class="mt-1 text-xs text-slate-500 dark:text-slate-400">确认手续费与到账余额并付款</p>
                </div>

                <!-- Price breakdown -->
                <div v-if="validAmount > 0" class="space-y-3 rounded-xl border border-slate-200/80 bg-slate-50/80 p-4 text-xs dark:border-white/5 dark:bg-white/[0.01]">
                  <div class="flex justify-between text-slate-500 dark:text-slate-400">
                    <span>充值面额</span>
                    <span class="font-semibold text-slate-950 dark:text-white">{{ formatSelectedPaymentAmount(validAmount) }}</span>
                  </div>
                  <div v-if="feeRate > 0" class="flex justify-between text-slate-500 dark:text-slate-400">
                    <span>手续费 ({{ feeRate }}%)</span>
                    <span class="font-semibold text-slate-950 dark:text-white">{{ formatSelectedPaymentAmount(feeAmount) }}</span>
                  </div>
                  <div class="flex justify-between border-b border-slate-200/80 pb-3 text-slate-500 dark:border-white/5 dark:text-slate-400">
                    <span>到账额度</span>
                    <span class="font-semibold text-emerald-600 dark:text-emerald-400">${{ creditedAmount.toFixed(2) }}</span>
                  </div>
                  <div class="flex justify-between pt-2 text-sm">
                    <span class="font-bold text-slate-700 dark:text-slate-200">实际应付</span>
                    <span class="text-xl font-extrabold text-primary-400">{{ formatSelectedPaymentAmount(totalAmount) }}</span>
                  </div>
                  <p v-if="balanceRechargeMultiplier !== 1" class="mt-2 border-t border-slate-200/80 pt-1 text-[10px] text-slate-500 dark:border-white/5">
                    充值比例折算：$1.00 到账额度需实付 ¥{{ balanceRechargeMultiplier.toFixed(2) }}
                  </p>
                </div>
                <div v-else class="rounded-xl border border-slate-200/80 bg-slate-50/80 p-6 text-center text-xs text-slate-500 dark:border-white/5 dark:bg-white/[0.01]">
                  请在左侧选择或输入充值金额来计算应付金额
                </div>

                <!-- Payment methods selector -->
                <div class="space-y-2">
                  <p class="text-xs font-bold text-slate-500 dark:text-slate-400">选择支付方式</p>
                  <div v-if="enabledMethods.length >= 1" class="grid grid-cols-2 gap-3">
                    <button
                      v-for="method in methodOptions"
                      :key="method.type"
                      type="button"
                      :disabled="!method.available"
                      :class="[
                        'relative flex flex-col items-center justify-center rounded-xl border p-4 text-center transition-all duration-200',
                        !method.available
                          ? 'cursor-not-allowed border-slate-200 bg-slate-50 opacity-40 dark:border-white/5 dark:bg-white/[0.01]'
                          : selectedMethod === method.type
                            ? 'border-primary-400 bg-primary-50 text-slate-950 shadow-[0_0_15px_rgba(20,184,166,0.15)] dark:border-primary-500 dark:bg-primary-500/10 dark:text-white'
                            : 'border-slate-200/80 bg-white text-slate-600 hover:border-primary-200 hover:text-slate-950 dark:border-white/5 dark:bg-slate-900/40 dark:text-slate-300 dark:hover:border-white/10 dark:hover:text-white'
                      ]"
                      @click="selectedMethod = method.type"
                    >
                      <span v-if="selectedMethod === method.type" class="absolute top-1.5 right-1.5 flex h-3.5 w-3.5 items-center justify-center rounded-full bg-primary-500 text-slate-950">
                        <Icon name="checkCircle" size="xs" />
                      </span>

                      <img :src="methodIconSrc(method.type)" :alt="paymentMethodLabel(method)" class="h-6 w-6 object-contain" />
                      <span class="text-xs font-bold mt-2">{{ paymentMethodLabel(method) }}</span>
                    </button>
                  </div>
                </div>

                <!-- Pay Button -->
                <button
                  type="button"
                  class="w-full flex h-12 items-center justify-center gap-2 rounded-xl text-sm font-extrabold text-slate-950 shadow-lg transition-all duration-200 active:scale-[0.99] disabled:cursor-not-allowed btn-shimmer disabled:bg-slate-800 disabled:text-slate-500 disabled:shadow-none"
                  :disabled="!canSubmit || submitting"
                  @click="handleSubmitRecharge"
                >
                  <span v-if="submitting" class="h-4 w-4 animate-spin rounded-full border-2 border-slate-950 border-t-transparent"></span>
                  <span v-if="!submitting">立即充值 {{ formatSelectedPaymentAmount(totalAmount) }}</span>
                  <span v-else>正在处理中...</span>
                  <Icon v-if="!submitting && canSubmit" name="arrowRight" size="xs" />
                </button>

                <!-- Trust info -->
                <div class="grid grid-cols-2 gap-3 border-t border-slate-200/80 pt-5 text-center text-[10px] text-slate-500 dark:border-white/5 dark:text-slate-400">
                  <span class="flex items-center justify-center gap-1.5 rounded-lg bg-slate-50 py-2 dark:bg-white/[0.01]">
                    <Icon name="shield" size="xs" class="text-primary-400" />
                    安全充值
                  </span>
                  <span class="flex items-center justify-center gap-1.5 rounded-lg bg-slate-50 py-2 dark:bg-white/[0.01]">
                    <Icon name="clock" size="xs" class="text-primary-400" />
                    即时入账
                  </span>
                </div>
              </aside>

            </div>
          </template>

          <!-- Help Info Section (Global bottom placement) -->
          <div v-if="(checkout.help_text || checkout.help_image_url) && !selectedPlan" class="mx-auto mt-16 max-w-5xl border-t border-slate-200/80 pt-8 dark:border-white/5">
            <div class="flex flex-col items-center justify-center gap-6 rounded-2xl border border-slate-200/80 bg-white/80 p-6 text-center shadow-sm md:flex-row md:text-left dark:border-white/5 dark:bg-slate-900/20">
              <img
                v-if="checkout.help_image_url"
                :src="checkout.help_image_url"
                alt="Help Image"
                class="h-28 max-w-full cursor-pointer rounded-xl object-contain shadow-md transition-opacity hover:opacity-[0.85]"
                @click="previewImage = checkout.help_image_url"
              />
              <div v-if="checkout.help_text" class="space-y-1">
                <h4 class="text-sm font-bold text-slate-950 dark:text-white">支付常见问题及解答</h4>
                <p class="max-w-xl text-xs leading-relaxed text-slate-500 dark:text-slate-400">{{ checkout.help_text }}</p>
              </div>
            </div>
          </div>

          <!-- Active subscriptions (Listed at bottom of Step 1) -->
          <div v-if="activeSubscriptions.length > 0 && !selectedPlan" class="mx-auto mt-12 max-w-5xl space-y-4">
            <h3 class="flex items-center gap-2 text-sm font-bold text-slate-500 dark:text-slate-400">
              <span class="inline-block w-1.5 h-3.5 bg-emerald-500 rounded-full"></span>
              您的当前有效订阅
            </h3>
            
            <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              <div
                v-for="sub in activeSubscriptions"
                :key="sub.id"
                class="flex items-center gap-3 rounded-xl border border-slate-200/80 bg-white/90 p-4 shadow-sm dark:border-white/5 dark:bg-[#090f1d]/40"
              >
                <div :class="['h-8 w-1 shrink-0 rounded-full', platformAccentBarClass(activeSubscriptionPlatform(sub))]" />
                <div class="min-w-0 flex-1 space-y-0.5">
                  <div class="flex items-center gap-2">
                    <span class="truncate text-xs font-bold text-slate-950 dark:text-white">
                      {{ activeSubscriptionDisplayName(sub) }}
                    </span>
                    <span :class="['shrink-0 rounded-full px-1.5 py-0.5 text-[8px] font-bold uppercase tracking-wider', platformBadgeLightClass(activeSubscriptionPlatform(sub))]">
                      {{ platformLabel(activeSubscriptionPlatform(sub)) }}
                    </span>
                  </div>
                  <div class="flex flex-wrap gap-x-3 text-[10px] text-slate-500 dark:text-slate-400">
                    <span>{{ activeSubscriptionQuotaText(sub) }}</span>
                    <span v-if="sub.expires_at">{{ formatSubscriptionRemaining(sub.expires_at) }}</span>
                    <span v-else>无到期时间</span>
                  </div>
                </div>
                <span class="rounded border border-emerald-200 bg-emerald-50 px-1.5 py-0.5 text-[9px] font-bold text-emerald-700 dark:border-emerald-500/20 dark:bg-emerald-500/10 dark:text-emerald-400">
                  有效中
                </span>
              </div>
            </div>
          </div>

        </template>
      </template>
    </div>

    <!-- Renewal Plan Selection Modal -->
    <Teleport to="body">
      <Transition name="modal">
        <div v-if="showRenewalModal" class="fixed inset-0 z-50 flex items-center justify-center bg-black/45 p-4 backdrop-blur-sm dark:bg-black/75" @click.self="closeRenewalModal">
          <div class="relative w-full max-w-lg rounded-2xl border border-slate-200/80 bg-white p-6 shadow-2xl dark:border-white/5 dark:bg-[#0b1222]">
            <!-- Close button -->
            <button class="absolute right-4 top-4 rounded-lg p-1.5 text-slate-500 transition-colors hover:bg-slate-100 hover:text-slate-950 dark:text-slate-400 dark:hover:bg-white/5 dark:hover:text-white" @click="closeRenewalModal">
              <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" /></svg>
            </button>
            <h3 class="mb-4 flex items-center gap-2 text-lg font-bold text-slate-950 dark:text-white">
              <span class="inline-block w-1 h-4 bg-primary-500 rounded-full"></span>
              {{ t('payment.selectPlan') }}
            </h3>
            <div class="space-y-4">
              <SubscriptionPlanCard v-for="plan in renewalPlans" :key="plan.id" :plan="plan" :active-subscriptions="activeSubscriptions" @select="selectPlanFromModal" />
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>

    <!-- Plan Groups Detail Modal -->
    <Teleport to="body">
      <Transition name="modal">
        <div
          v-if="planGroupsModalPlan"
          class="fixed inset-0 z-[65] flex items-center justify-center bg-slate-950/70 p-4 backdrop-blur-sm"
          @click.self="closePlanGroupsModal"
        >
          <div class="relative flex max-h-[82vh] w-full max-w-2xl flex-col overflow-hidden rounded-2xl border border-slate-200/80 bg-white shadow-2xl dark:border-white/10 dark:bg-[#0b1222]">
            <div class="flex items-start justify-between gap-4 border-b border-slate-200/80 p-5 dark:border-white/10">
              <div class="min-w-0">
                <p class="text-xs font-bold uppercase tracking-wide text-primary-600 dark:text-primary-300">
                  覆盖分组
                </p>
                <h3 class="mt-1 truncate text-lg font-extrabold text-slate-950 dark:text-white">
                  {{ planGroupsModalPlan.name }}
                </h3>
                <p class="mt-1 text-xs text-slate-500 dark:text-slate-400">
                  共 {{ planGroupCount(planGroupsModalPlan) }} 个分组，展示每个分组的倍率与倍率后实际单价
                </p>
              </div>
              <button
                type="button"
                class="rounded-lg p-1.5 text-slate-500 transition hover:bg-slate-100 hover:text-slate-950 dark:text-slate-400 dark:hover:bg-white/5 dark:hover:text-white"
                @click="closePlanGroupsModal"
              >
                <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>
            <div class="flex-1 space-y-2 overflow-y-auto p-5 group-scrollbar">
              <div
                v-for="group in getPlanView(planGroupsModalPlan?.id).groupRows"
                :key="group.key"
                class="grid grid-cols-[minmax(0,1fr)_auto] gap-3 rounded-xl border border-slate-200/80 bg-slate-50/90 p-3 dark:border-white/5 dark:bg-slate-950/40"
              >
                <div class="min-w-0">
                  <div class="truncate text-sm font-bold text-slate-950 dark:text-white">{{ group.name }}</div>
                  <div class="mt-1 flex flex-wrap items-center gap-2 text-[11px] font-medium text-slate-500 dark:text-slate-400">
                    <template v-for="(item, idx) in group.metadataLines" :key="idx">
                      <span v-if="idx > 0">·</span>
                      <span :class="{ 'font-semibold text-emerald-600 dark:text-emerald-400': idx === group.metadataLines.length - 1 }">
                        {{ item }}
                      </span>
                    </template>
                  </div>
                </div>
                <span class="h-fit rounded-md border border-emerald-200 bg-emerald-50 px-2 py-1 text-xs font-bold text-emerald-700 dark:border-emerald-500/25 dark:bg-emerald-500/10 dark:text-emerald-300">
                  {{ group.multiplierText }}
                </span>
              </div>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>

    <!-- Image Preview Overlay -->
    <Teleport to="body">
      <Transition name="modal">
        <div v-if="previewImage" class="fixed inset-0 z-[60] flex items-center justify-center bg-black/85 backdrop-blur-sm" @click="previewImage = ''">
          <img :src="previewImage" alt="" class="max-h-[85vh] max-w-[90vw] rounded-xl object-contain shadow-2xl border border-white/5" />
        </div>
      </Transition>
    </Teleport>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { usePaymentStore } from '@/stores/payment'
import { useSubscriptionStore } from '@/stores/subscriptions'
import { useAppStore } from '@/stores'
import { paymentAPI } from '@/api/payment'
import { extractApiErrorMessage, extractI18nErrorMessage } from '@/utils/apiError'
import { isMobileDevice } from '@/utils/device'
import type { SubscriptionPlan, SubscriptionPlanGroupInfo, CheckoutInfoResponse, CreateOrderResult, OrderType } from '@/types/payment'
import { hasPeakRate, formatPeakRateWindow, serverTimezoneLabel } from '@/utils/peak-rate'
import AppLayout from '@/components/layout/AppLayout.vue'
import AmountInput from '@/components/payment/AmountInput.vue'
import { METHOD_ORDER, getPaymentPopupFeatures } from '@/components/payment/providerConfig'
import {
  PAYMENT_RECOVERY_STORAGE_KEY,
  buildCreateOrderPayload,
  clearPaymentRecoverySnapshot,
  decidePaymentLaunch,
  getVisibleMethods,
  normalizeVisibleMethod,
  readPaymentRecoverySnapshot,
  type PaymentRecoverySnapshot,
  writePaymentRecoverySnapshot,
} from '@/components/payment/paymentFlow'
import { platformAccentBarClass, platformBadgeLightClass, platformLabel } from '@/utils/platformColors'
import { subscriptionDisplayName } from '@/utils/subscriptionPlanDisplay'
import SubscriptionPlanCard from '@/components/payment/SubscriptionPlanCard.vue'
import PaymentStatusPanel from '@/components/payment/PaymentStatusPanel.vue'
import Icon from '@/components/icons/Icon.vue'
import { DEFAULT_PAYMENT_CURRENCY, formatPaymentAmount, normalizePaymentCurrency } from '@/components/payment/currency'
import { planValiditySuffix } from '@/components/payment/validity'
import type { PaymentMethodOption } from '@/components/payment/PaymentMethodSelector.vue'
import type { UserSubscription } from '@/types'
import { buildPaymentErrorToastMessage, describePaymentScenarioError, paymentMethodI18nKey } from './paymentUx'
import { hasWechatResumeQuery, parseWechatResumeRoute, stripWechatResumeQuery } from './paymentWechatResume'
import { formatRemainingDurationCompact } from '@/utils/subscriptionTime'
import { sortGroupsForDisplay } from '@/utils/groupDisplayOrder'
import alipayIcon from '@/assets/icons/alipay.svg'
import wxpayIcon from '@/assets/icons/wxpay.svg'
import stripeIcon from '@/assets/icons/stripe.svg'
import airwallexIcon from '@/assets/icons/airwallex.svg'
import paymentIcon from '@/assets/icons/payment.svg'


const i18n = useI18n()
const { t } = i18n
const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const paymentStore = usePaymentStore()
const subscriptionStore = useSubscriptionStore()
const appStore = useAppStore()

const user = computed(() => authStore.user)
const activeSubscriptions = computed(() => subscriptionStore.activeSubscriptions)

function formatSubscriptionRemaining(expiresAt: string): string {
  return formatRemainingDurationCompact(expiresAt) || t('userSubscriptions.status.expired')
}

function positiveSubscriptionLimit(value: number | null | undefined): number | null {
  return typeof value === 'number' && value > 0 ? value : null
}

function activeSubscriptionPlatform(subscription: UserSubscription): string {
  return subscription.groups?.[0]?.platform || subscription.group?.platform || ''
}

function activeSubscriptionDisplayName(subscription: UserSubscription): string {
  return subscriptionDisplayName(subscription, subscriptionPlans.value)
}

function activeSubscriptionQuotaMetric(subscription: UserSubscription): QuotaMetric {
  const monthlyLimit = positiveSubscriptionLimit(subscription.monthly_limit_usd)
  if (monthlyLimit) return { key: 'monthly', value: monthlyLimit }

  const weeklyLimit = positiveSubscriptionLimit(subscription.weekly_limit_usd)
  if (weeklyLimit) return { key: 'weekly', value: weeklyLimit }

  const dailyLimit = positiveSubscriptionLimit(subscription.daily_limit_usd)
  if (dailyLimit) return { key: 'daily', value: dailyLimit }

  if (subscription.entitlement_id != null || subscription.plan_id != null) return null

  const legacyMonthlyLimit = positiveSubscriptionLimit(subscription.group?.monthly_limit_usd)
  if (legacyMonthlyLimit) return { key: 'monthly', value: legacyMonthlyLimit }

  const legacyWeeklyLimit = positiveSubscriptionLimit(subscription.group?.weekly_limit_usd)
  if (legacyWeeklyLimit) return { key: 'weekly', value: legacyWeeklyLimit }

  const legacyDailyLimit = positiveSubscriptionLimit(subscription.group?.daily_limit_usd)
  if (legacyDailyLimit) return { key: 'daily', value: legacyDailyLimit }

  return null
}

function activeSubscriptionQuotaText(subscription: UserSubscription): string {
  const quota = activeSubscriptionQuotaMetric(subscription)
  if (!quota) return '额度: 无限制'

  const suffix = quota.key === 'monthly' ? '月' : quota.key === 'weekly' ? '周' : '天'
  return `额度: $${formatPlainNumber(quota.value)} / ${suffix}`
}

const loading = ref(true)
const submitting = ref(false)
const errorMessage = ref('')
const errorHintMessage = ref('')
const activeTab = ref<'recharge' | 'subscription'>('subscription')
const amount = ref<number | null>(null)
const selectedMethod = ref('')
const selectedPlan = ref<SubscriptionPlan | null>(null)
const previewImage = ref('')
const isComparisonOpen = ref(true)
const planGroupsModalPlan = ref<SubscriptionPlan | null>(null)

const paymentPhase = ref<'select' | 'paying'>('select')

interface CreateOrderOptions {
  openid?: string
  wechatResumeToken?: string
  paymentType?: string
  isResume?: boolean
  mobileQrFallbackAttempted?: boolean
}

interface WeixinJSBridgeLike {
  invoke(
    action: string,
    payload: Record<string, unknown>,
    callback: (result: Record<string, unknown>) => void,
  ): void
}

function emptyPaymentState(): PaymentRecoverySnapshot {
  return {
    orderId: 0,
    amount: 0,
    qrCode: '',
    expiresAt: '',
    paymentType: '',
    payUrl: '',
    outTradeNo: '',
    clientSecret: '',
    intentId: '',
    currency: '',
    countryCode: '',
    paymentEnv: '',
    payAmount: 0,
    orderType: '',
    paymentMode: '',
    resumeToken: '',
    createdAt: 0,
  }
}

function getWeixinJSBridge(): WeixinJSBridgeLike | undefined {
  return (window as Window & { WeixinJSBridge?: WeixinJSBridgeLike }).WeixinJSBridge
}

function waitForWeixinJSBridge(timeoutMs = 4000): Promise<WeixinJSBridgeLike | null> {
  const existing = getWeixinJSBridge()
  if (existing) return Promise.resolve(existing)

  return new Promise((resolve) => {
    let settled = false
    const finish = (bridge: WeixinJSBridgeLike | null) => {
      if (settled) return
      settled = true
      document.removeEventListener('WeixinJSBridgeReady', handleReady)
      document.removeEventListener('onWeixinJSBridgeReady', handleReady)
      window.clearTimeout(timer)
      resolve(bridge)
    }
    const handleReady = () => finish(getWeixinJSBridge() ?? null)
    const timer = window.setTimeout(() => finish(getWeixinJSBridge() ?? null), timeoutMs)
    document.addEventListener('WeixinJSBridgeReady', handleReady, false)
    document.addEventListener('onWeixinJSBridgeReady', handleReady, false)
  })
}

async function invokeWechatJsapiPayment(payload: Record<string, unknown>): Promise<Record<string, unknown>> {
  const bridge = await waitForWeixinJSBridge()
  if (!bridge) {
    throw new Error('WECHAT_JSAPI_UNAVAILABLE')
  }
  return new Promise((resolve) => {
    bridge.invoke('getBrandWCPayRequest', payload, (result) => resolve(result || {}))
  })
}

const paymentState = ref<PaymentRecoverySnapshot>(emptyPaymentState())

function persistRecoverySnapshot(snapshot: PaymentRecoverySnapshot) {
  if (typeof window === 'undefined' || !snapshot.orderId) return
  writePaymentRecoverySnapshot(window.localStorage, snapshot, PAYMENT_RECOVERY_STORAGE_KEY)
}

function removeRecoverySnapshot() {
  if (typeof window === 'undefined') return
  clearPaymentRecoverySnapshot(window.localStorage, PAYMENT_RECOVERY_STORAGE_KEY)
}

function resetPayment() {
  paymentPhase.value = 'select'
  paymentState.value = emptyPaymentState()
  removeRecoverySnapshot()
}

async function redirectToPaymentResult(state: PaymentRecoverySnapshot): Promise<void> {
  const query: Record<string, string | undefined> = {}
  if (state.orderId > 0) {
    query.order_id = String(state.orderId)
  }
  if (state.outTradeNo) {
    query.out_trade_no = state.outTradeNo
  }
  if (state.resumeToken) {
    query.resume_token = state.resumeToken
  }
  await router.push({
    path: '/payment/result',
    query,
  })
}

function buildWechatOAuthAuthorizeUrl(
  authorizeUrl: string,
  context: { paymentType: string; orderType: OrderType; planId?: number; orderAmount: number },
): string {
  const normalizedUrl = authorizeUrl.trim()
  if (!normalizedUrl || typeof window === 'undefined') {
    return normalizedUrl
  }

  try {
    const targetUrl = new URL(normalizedUrl, window.location.origin)
    const redirectPath = targetUrl.searchParams.get('redirect') || '/purchase'
    const redirectUrl = new URL(redirectPath, window.location.origin)
    const paymentType = normalizeVisibleMethod(context.paymentType) || context.paymentType.trim() || 'wxpay'

    redirectUrl.searchParams.set('payment_type', paymentType)
    redirectUrl.searchParams.set('order_type', context.orderType)

    if (context.planId) {
      redirectUrl.searchParams.set('plan_id', String(context.planId))
    } else {
      redirectUrl.searchParams.delete('plan_id')
    }

    if (context.orderAmount > 0) {
      redirectUrl.searchParams.set('amount', String(context.orderAmount))
    } else {
      redirectUrl.searchParams.delete('amount')
    }

    targetUrl.searchParams.set('redirect', `${redirectUrl.pathname}${redirectUrl.search}`)
    return targetUrl.toString()
  } catch {
    return normalizedUrl
  }
}

function onPaymentDone() {
  const wasSubscription = paymentState.value.orderType === 'subscription'
  resetPayment()
  selectedPlan.value = null
  if (wasSubscription) {
    subscriptionStore.fetchActiveSubscriptions(true).catch(() => {})
  }
}

function onPaymentSuccess() {
  removeRecoverySnapshot()
  authStore.refreshUser()
  if (paymentState.value.orderType === 'subscription') {
    subscriptionStore.fetchActiveSubscriptions(true).catch(() => {})
  }
}

function onPaymentSettled() {
  removeRecoverySnapshot()
}

// All checkout data from single API call
const checkout = ref<CheckoutInfoResponse>({
  methods: {}, global_min: 0, global_max: 0,
  plans: [], balance_disabled: false, balance_recharge_multiplier: 1, subscription_usd_to_cny_rate: 0, recharge_fee_rate: 0, help_text: '', help_image_url: '', stripe_publishable_key: '',
})

const tabs = computed(() => {
  const result: { key: 'recharge' | 'subscription'; label: string }[] = []
  result.push({ key: 'subscription', label: t('payment.tabSubscribe') })
  if (!checkout.value.balance_disabled) result.push({ key: 'recharge', label: t('payment.tabTopUp') })
  return result
})

const visibleMethods = computed(() => getVisibleMethods(checkout.value.methods))
const enabledMethods = computed(() => Object.keys(visibleMethods.value))
const validAmount = computed(() => amount.value ?? 0)
const balanceRechargeMultiplier = computed(() => {
  const multiplier = checkout.value.balance_recharge_multiplier
  return multiplier > 0 ? multiplier : 1
})
// 订阅 CNY 换算汇率（1 USD = X CNY）。0 = 未配置，订阅保持 price 直付（与后端 opt-in 条件严格镜像）。
const subscriptionUsdToCnyRate = computed(() => {
  const rate = checkout.value.subscription_usd_to_cny_rate
  return Number.isFinite(rate) && rate > 0 ? rate : 0
})
const creditedAmount = computed(() => Math.round((validAmount.value * balanceRechargeMultiplier.value) * 100) / 100)

const subscriptionPlans = computed(() =>
  [...checkout.value.plans].sort((a, b) => {
    const orderDiff = (a.sort_order || 0) - (b.sort_order || 0)
    if (orderDiff !== 0) return orderDiff
    return a.price - b.price
  })
)

const planViewMap = computed<Record<number, PlanView>>(() => {
  const map: Record<number, PlanView> = {}
  for (const plan of subscriptionPlans.value) {
    const groupRows = buildGroupRows(plan)
    map[plan.id] = {
      groupRows,
      cardGroups: groupRows.slice(0, PLAN_CARD_GROUP_LIMIT),
      cardOverflowCount: Math.max(0, groupRows.length - PLAN_CARD_GROUP_LIMIT),
      compareGroups: groupRows.slice(0, PLAN_COMPARE_GROUP_LIMIT),
      compareOverflowCount: Math.max(0, groupRows.length - PLAN_COMPARE_GROUP_LIMIT),
    }
  }
  return map
})

const PLAN_CARD_GROUP_LIMIT = 3
const PLAN_COMPARE_GROUP_LIMIT = 4

const EMPTY_PLAN_VIEW: PlanView = {
  groupRows: [],
  cardGroups: [],
  cardOverflowCount: 0,
  compareGroups: [],
  compareOverflowCount: 0,
}

function getPlanView(planId?: number | null): PlanView {
  if (!planId) return EMPTY_PLAN_VIEW
  return planViewMap.value[planId] || EMPTY_PLAN_VIEW
}

function isRecommendedPlan(plan: SubscriptionPlan): boolean {
  return /尊享|推荐|优选|premium|pro/i.test(plan.name)
}

const recommendedPlan = computed(() =>
  subscriptionPlans.value.find(isRecommendedPlan)
    ?? subscriptionPlans.value[Math.min(2, subscriptionPlans.value.length - 1)]
    ?? null
)

const selectedCheckoutPlan = computed(() =>
  selectedPlan.value
    ?? recommendedPlan.value
    ?? subscriptionPlans.value[0]
    ?? null
)


const selectedPlanSummaryText = computed(() => {
  const plan = selectedCheckoutPlan.value
  if (!plan) return ''
  return `${planQuotaText(plan)} / ${validitySuffixForPlan(plan)}有效`
})

function uniqueStrings(values: string[]): string[] {
  return [...new Set(values.map(value => value.trim()).filter(Boolean))]
}

function planGroups(plan: SubscriptionPlan): SubscriptionPlanGroupInfo[] {
  if (Array.isArray(plan.groups) && plan.groups.length > 0) {
    return sortGroupsForDisplay(plan.groups)
  }
  if (!plan.group_name) return []
  return [{
    id: plan.group_id,
    platform: plan.group_platform || '',
    name: plan.group_name,
    rate_multiplier: plan.rate_multiplier ?? 1,
    daily_limit_usd: plan.daily_limit_usd ?? null,
    weekly_limit_usd: plan.weekly_limit_usd ?? null,
    monthly_limit_usd: plan.monthly_limit_usd ?? null,
    supported_model_scopes: plan.supported_model_scopes || [],
    sort_order: 0,
  }]
}

function displayGroupName(name: string): string {
  return name
    .replace(/^codex[-_]?/i, 'Codex ')
    .replace(/^claude订阅组[-_]?/i, 'Claude ')
    .replace(/^claude[-_]?/i, 'Claude ')
    .replace(/^kiro[-_]?/i, 'Kiro ')
    .replace(/^krio[-_]?/i, 'Kiro ')
    .replace(/测试分组/g, '测试')
    .trim()
}

const GROUP_LABELS = {
  platform: '平台',
  quota: '额度',
  actualCost: '倍率后实际单价',
  unlimitedQuota: '不限',
  allPlatforms: '全部适用',
}

interface PlanGroupViewRow {
  key: string
  name: string
  multiplierText: string
  platform: string
  actualCostText: string | null
  metadataLines: string[]
}

interface PlanView {
  groupRows: PlanGroupViewRow[]
  cardGroups: PlanGroupViewRow[]
  cardOverflowCount: number
  compareGroups: PlanGroupViewRow[]
  compareOverflowCount: number
}

function formatGroupMultiplier(value?: number | null): string {
  const multiplier = value ?? 1
  const fractionDigits = Math.abs(multiplier - Math.round(multiplier)) > 0.0001 ? 2 : 0
  return `x${formatPlainNumber(multiplier, fractionDigits)}`
}

function groupQuotaText(group: SubscriptionPlanGroupInfo): string {
  if (typeof group.monthly_limit_usd === 'number' && group.monthly_limit_usd > 0) return `$${formatPlainNumber(group.monthly_limit_usd)}/月`
  if (typeof group.weekly_limit_usd === 'number' && group.weekly_limit_usd > 0) return `$${formatPlainNumber(group.weekly_limit_usd)}/周`
  if (typeof group.daily_limit_usd === 'number' && group.daily_limit_usd > 0) return `$${formatPlainNumber(group.daily_limit_usd)}/天`
  return ''
}

function planGroupLabels(plan: SubscriptionPlan, limit = Number.POSITIVE_INFINITY): string[] {
  const groups = planGroups(plan)
  if (groups.length > 0) {
    return groups.map(group => displayGroupName(group.name)).slice(0, limit)
  }
  if (plan.allowed_platforms?.length) {
    return plan.allowed_platforms.map(platform => platformLabel(platform)).slice(0, limit)
  }
  if (plan.access_scope === 'all_subscription_groups') return ['全部订阅分组']
  if (plan.access_scope === 'platform_subscription_groups') return ['平台订阅分组']
  return ['指定分组']
}

function planGroupCount(plan: SubscriptionPlan): number {
  const groupCount = planGroups(plan).length
  if (groupCount > 0) return groupCount
  if (plan.group_ids?.length) return plan.group_ids.length
  if (plan.allowed_platforms?.length) return plan.allowed_platforms.length
  return 1
}

function planGroupCountText(plan: SubscriptionPlan): string {
  return `${planGroupCount(plan)} 个分组`
}

function buildGroupRows(plan: SubscriptionPlan): PlanGroupViewRow[] {
  const groups = planGroups(plan)
  const baseCost = unitCost(plan)
  
  let rawItems: {
    key: string
    name: string
    multiplier: number
    platform: string
    quotaText: string
    actualCost: number | null
  }[] = []

  if (groups.length > 0) {
    rawItems = groups.map(group => {
      const multiplier = group.rate_multiplier ?? 1
      return {
        key: String(group.id),
        name: displayGroupName(group.name),
        multiplier,
        platform: group.platform || '',
        quotaText: groupQuotaText(group),
        actualCost: baseCost !== null ? baseCost * multiplier : null,
      }
    })
  } else {
    const fallbackMultiplier = plan.rate_multiplier ?? 1
    rawItems = planGroupLabels(plan).map((name, index) => ({
      key: `${plan.id}-${name}-${index}`,
      name,
      multiplier: fallbackMultiplier,
      platform: '',
      quotaText: '',
      actualCost: baseCost !== null ? baseCost * fallbackMultiplier : null,
    }))
  }

  return rawItems.map(item => {
    const multiplierText = formatGroupMultiplier(item.multiplier)
    const actualCostText = item.actualCost !== null 
      ? `¥${formatPlainNumber(item.actualCost, item.actualCost > 0 && item.actualCost < 1 ? 4 : 2)}/刀`
      : null

    const platformText = item.platform ? platformLabel(item.platform) : GROUP_LABELS.allPlatforms
    const metadataLines = [
      `${GROUP_LABELS.platform}: ${platformText}`
    ]
    if (item.quotaText) {
      metadataLines.push(`${GROUP_LABELS.quota}: ${item.quotaText}`)
    }
    if (actualCostText) {
      metadataLines.push(`${GROUP_LABELS.actualCost}: ${actualCostText}`)
    }

    return {
      key: item.key,
      name: item.name,
      multiplierText,
      platform: item.platform,
      actualCostText,
      metadataLines
    }
  })
}

function openPlanGroupsModal(plan: SubscriptionPlan) {
  planGroupsModalPlan.value = plan
}

function closePlanGroupsModal() {
  planGroupsModalPlan.value = null
}

function planCapabilityTags(plan: SubscriptionPlan): string[] {
  const groups = planGroups(plan)
  const searchText = [
    plan.name,
    plan.description,
    ...(plan.features || []),
    ...groups.map(group => `${group.platform} ${group.name}`),
  ].join(' ').toLowerCase()
  const scopes = uniqueStrings([
    ...(plan.supported_model_scopes || []),
    ...groups.flatMap(group => group.supported_model_scopes || []),
  ])

  const tags: string[] = []
  if (searchText.includes('openai') || searchText.includes('codex') || plan.allowed_platforms?.includes('openai')) tags.push('OpenAI / Codex')
  if (searchText.includes('claude') || searchText.includes('anthropic')) tags.push('Claude Code')
  if (scopes.some(scope => scope.includes('gemini'))) tags.push('Gemini')
  if (searchText.includes('/v1/messages') || searchText.includes('messages')) tags.push('/v1/messages')
  if (plan.access_scope === 'all_subscription_groups') tags.push('全量分组')
  if (plan.access_scope === 'platform_subscription_groups') tags.push('平台分组')
  tags.push('原倍率计费')
  return uniqueStrings(tags).slice(0, 6)
}


const heroCapabilityChips = computed(() => {
  const tags = subscriptionPlans.value.flatMap(planCapabilityTags)
  return uniqueStrings([...tags, '/v1/messages', '全模型覆盖']).slice(0, 7)
})


// Check if an amount fits a method's [min, max]. 0 = no limit.
function amountFitsMethod(amt: number, methodType: string): boolean {
  if (amt <= 0) return true
  const ml = visibleMethods.value[methodType]
  if (!ml) return false
  if (ml.single_min > 0 && amt < ml.single_min) return false
  if (ml.single_max > 0 && amt > ml.single_max) return false
  return true
}

// Visible methods decide the amount range shown to users.
const globalMinAmount = computed(() => {
  const limits = Object.values(visibleMethods.value)
  if (limits.length === 0) return 0
  if (limits.some(limit => limit.single_min <= 0)) return 0
  return Math.min(...limits.map(limit => limit.single_min))
})
const globalMaxAmount = computed(() => {
  const limits = Object.values(visibleMethods.value)
  if (limits.length === 0) return 0
  if (limits.some(limit => limit.single_max <= 0)) return 0
  return Math.max(...limits.map(limit => limit.single_max))
})

// Selected method's limits (for validation and error messages)
const selectedLimit = computed(() => visibleMethods.value[selectedMethod.value])
const selectedCurrency = computed(() => normalizePaymentCurrency(selectedLimit.value?.currency))
const localeCode = computed(() => {
  const raw = i18n.locale as unknown
  if (typeof raw === 'string') return raw
  if (raw && typeof raw === 'object' && 'value' in raw) {
    return String((raw as { value?: string }).value || '')
  }
  return undefined
})

function currencyFractionDigits(currency: string): number {
  try {
    return new Intl.NumberFormat(undefined, {
      style: 'currency',
      currency,
    }).resolvedOptions().maximumFractionDigits ?? 2
  } catch {
    return 2
  }
}

function roundPaymentAmount(value: number, currency: string): number {
  if (!Number.isFinite(value)) return 0
  const factor = 10 ** currencyFractionDigits(currency)
  return Math.round(value * factor) / factor
}

function ceilPaymentAmount(value: number, currency: string): number {
  if (!Number.isFinite(value)) return 0
  const factor = 10 ** currencyFractionDigits(currency)
  return Math.ceil(value * factor) / factor
}

function subscriptionPaymentAmountForCurrency(value: number, currency: string): number {
  const rate = subscriptionUsdToCnyRate.value
  if (rate <= 0 || currency !== DEFAULT_PAYMENT_CURRENCY) return roundPaymentAmount(value, currency)
  return roundPaymentAmount(value * rate, currency)
}

function formatSelectedPaymentAmount(value: number): string {
  return formatPaymentAmount(value, selectedCurrency.value, localeCode.value)
}

type QuotaMetric = { key: 'daily' | 'weekly' | 'monthly'; value: number } | null

function formatPlainNumber(value: number, fractionDigits = 0): string {
  return new Intl.NumberFormat(localeCode.value, {
    maximumFractionDigits: fractionDigits,
    minimumFractionDigits: 0,
  }).format(value)
}

function formatPlanPrice(plan: SubscriptionPlan): string {
  return formatSelectedSubscriptionPaymentAmount(plan.price).replace(/\.00$/, '')
}

function quotaMetric(plan: SubscriptionPlan): QuotaMetric {
  if (plan.monthly_limit_usd != null) return { key: 'monthly', value: plan.monthly_limit_usd }
  if (plan.weekly_limit_usd != null) return { key: 'weekly', value: plan.weekly_limit_usd }
  if (plan.daily_limit_usd != null) return { key: 'daily', value: plan.daily_limit_usd }
  return null
}

function planQuotaText(plan: SubscriptionPlan): string {
  const quota = quotaMetric(plan)
  if (!quota) return t('payment.planCard.unlimited')
  return `$${formatPlainNumber(quota.value)}`
}

function unitCost(plan: SubscriptionPlan): number | null {
  const quota = quotaMetric(plan)?.value
  if (!quota || quota <= 0 || plan.price <= 0) return null
  return plan.price / quota
}

function unitCostText(plan: SubscriptionPlan): string {
  const cost = unitCost(plan)
  if (cost == null) return '-'
  return `¥${formatPlainNumber(cost, cost > 0 && cost < 1 ? 4 : 2)}/刀`
}

function discountText(plan: SubscriptionPlan): string {
  if (!plan.original_price || plan.original_price <= plan.price) return ''
  const percentage = Math.round((1 - plan.price / plan.original_price) * 100)
  return percentage > 0 ? `-${percentage}%` : ''
}

function planAudience(plan: SubscriptionPlan): string {
  if (/轻享|starter|basic/i.test(plan.name)) return '入门体验，轻松上手'
  if (/标准|standard/i.test(plan.name)) return '高稳定性，持续可用'
  if (/畅享|plus/i.test(plan.name)) return '稳定使用，性价之选'
  if (/尊享|premium|pro/i.test(plan.name)) return '高频使用，最佳选择'
  if (/定制|enterprise|custom/i.test(plan.name)) return '大额需求，专属定制'
  return '灵活调用，按需选择'
}

function compactPlanFeatures(plan: SubscriptionPlan): string[] {
  const features = (plan.features || [])
    .map(feature => feature.trim())
    .filter(feature => feature && feature !== '[]')
  if (features.length > 0) return features.slice(0, 5)
  if (/定制|enterprise|custom/i.test(plan.name)) {
    return ['全模型覆盖', '专属资源保障', '售后与技术支持', '企业合作']
  }
  return ['全模型覆盖', '标准接口调度', '原倍率计费', '可开票']
}

function normalizePlanText(value?: string | null): string {
  return (value || '').replace(/\s+/g, ' ').trim()
}

function planDescriptionText(plan: SubscriptionPlan): string {
  const description = normalizePlanText(plan.description)
  return description === '[]' ? '' : description
}

function compactPlanFeatureSummary(plan: SubscriptionPlan): string {
  return compactPlanFeatures(plan).slice(0, 4).join(' · ')
}

function planCardSummaryText(plan: SubscriptionPlan): string {
  return planDescriptionText(plan) || compactPlanFeatureSummary(plan)
}

function validitySuffixForPlan(plan: SubscriptionPlan): string {
  return planValiditySuffix(plan, t)
}
function paymentMethodLabel(method: string | PaymentMethodOption): string {
  if (typeof method !== 'string' && method.display_name) return method.display_name
  const type = typeof method === 'string' ? method : method.type
  return t(paymentMethodI18nKey(type))
}

function methodIconSrc(type: string): string {
  if (type.includes('alipay')) return alipayIcon
  if (type.includes('wxpay')) return wxpayIcon
  if (type === 'stripe') return stripeIcon
  if (type === 'airwallex') return airwallexIcon
  return paymentIcon
}

function formatSelectedSubscriptionPaymentAmount(value: number): string {
  return formatSelectedPaymentAmount(subscriptionPaymentAmountForCurrency(value, selectedCurrency.value))
}

const methodOptions = computed<PaymentMethodOption[]>(() =>
  enabledMethods.value.map((type) => {
    const ml = visibleMethods.value[type]
    return {
      type,
      display_name: ml?.display_name,
      fee_rate: ml?.fee_rate ?? 0,
      available: ml?.available !== false && amountFitsMethod(validAmount.value, type),
    }
  })
)

const feeRate = computed(() => checkout.value?.recharge_fee_rate ?? 0)
const feeAmount = computed(() =>
  feeRate.value > 0 && validAmount.value > 0
    ? Math.ceil(((validAmount.value * feeRate.value) / 100) * 100) / 100
    : 0
)
const totalAmount = computed(() =>
  feeRate.value > 0 && validAmount.value > 0
    ? Math.round((validAmount.value + feeAmount.value) * 100) / 100
    : validAmount.value
)

const amountError = computed(() => {
  if (validAmount.value <= 0) return ''
  // No method can handle this amount
  if (!enabledMethods.value.some((m) => amountFitsMethod(validAmount.value, m))) {
    return t('payment.amountNoMethod')
  }
  // Selected method can't handle this amount (but others can)
  const ml = selectedLimit.value
  if (ml) {
    if (ml.single_min > 0 && validAmount.value < ml.single_min) return t('payment.amountTooLow', { min: formatSelectedPaymentAmount(ml.single_min) })
    if (ml.single_max > 0 && validAmount.value > ml.single_max) return t('payment.amountTooHigh', { max: formatSelectedPaymentAmount(ml.single_max) })
  }
  return ''
})

const canSubmit = computed(() =>
  validAmount.value > 0
    && amountFitsMethod(validAmount.value, selectedMethod.value)
    && selectedLimit.value?.available !== false
)

const subPaymentAmount = computed(() => {
  const price = selectedCheckoutPlan.value?.price ?? 0
  return subscriptionPaymentAmountForCurrency(price, selectedCurrency.value)
})

const subFeeAmount = computed(() => {
  if (feeRate.value <= 0 || subPaymentAmount.value <= 0) return 0
  return ceilPaymentAmount((subPaymentAmount.value * feeRate.value) / 100, selectedCurrency.value)
})

const subTotalAmount = computed(() => {
  if (feeRate.value <= 0 || subPaymentAmount.value <= 0) return subPaymentAmount.value
  return roundPaymentAmount(subPaymentAmount.value + subFeeAmount.value, selectedCurrency.value)
})

function subscriptionTotalAmountForCurrency(value: number, currency: string): number {
  const paymentAmount = subscriptionPaymentAmountForCurrency(value, currency)
  if (feeRate.value <= 0 || paymentAmount <= 0) return paymentAmount
  const fee = ceilPaymentAmount((paymentAmount * feeRate.value) / 100, currency)
  return roundPaymentAmount(paymentAmount + fee, currency)
}

// Subscription-specific: method options based on gateway pay amount
const subMethodOptions = computed<PaymentMethodOption[]>(() => {
  const planPrice = selectedCheckoutPlan.value?.price ?? 0
  return enabledMethods.value.map((type) => {
    const ml = visibleMethods.value[type]
    const currency = normalizePaymentCurrency(ml?.currency)
    return {
      type,
      display_name: ml?.display_name,
      fee_rate: ml?.fee_rate ?? 0,
      available: ml?.available !== false && amountFitsMethod(subscriptionTotalAmountForCurrency(planPrice, currency), type),
    }
  })
})

const canSubmitSubscription = computed(() => {
  const plan = selectedCheckoutPlan.value
  return plan !== null
    && amountFitsMethod(subTotalAmount.value, selectedMethod.value)
    && selectedLimit.value?.available !== false
})

const subscriptionCtaLabel = computed(() => {
  if (submitting.value) return t('common.processing')
  if (enabledMethods.value.length === 0) return '支付方式暂未开放'
  if (!selectedMethod.value) return '选择支付方式'
  return `确认支付 ${formatSelectedPaymentAmount(subTotalAmount.value)}`
})

// Auto-switch to first available method when current selection can't handle the amount
watch(() => [validAmount.value, selectedMethod.value] as const, ([amt, method]) => {
  if (amt <= 0 || amountFitsMethod(amt, method)) return
  const available = enabledMethods.value.find((m) => amountFitsMethod(amt, m))
  if (available) selectedMethod.value = available
})

watch(() => [selectedCheckoutPlan.value?.price ?? 0, selectedMethod.value] as const, ([price, method]) => {
  const methodCurrency = normalizePaymentCurrency(visibleMethods.value[method]?.currency)
  const methodAmount = subscriptionTotalAmountForCurrency(price, methodCurrency)
  if (price <= 0 || amountFitsMethod(methodAmount, method)) return
  const available = enabledMethods.value.find((m) => {
    const currency = normalizePaymentCurrency(visibleMethods.value[m]?.currency)
    return amountFitsMethod(subscriptionTotalAmountForCurrency(price, currency), m)
  })
  if (available) selectedMethod.value = available
})

// Renewal modal state
const showRenewalModal = ref(false)
const renewGroupId = ref<number | null>(null)
const renewalPlans = computed(() => {
  if (renewGroupId.value == null) return []
  return checkout.value.plans.filter(p => p.group_id === renewGroupId.value)
})

function setActiveTab(tab: 'recharge' | 'subscription') {
  activeTab.value = tab
  selectedPlan.value = null
  errorMessage.value = ''
  errorHintMessage.value = ''
  paymentPhase.value = 'select'
  closePlanGroupsModal()
  closeRenewalModal()
}

function planHasPeakRate(plan: SubscriptionPlan): boolean {
  return hasPeakRate(plan)
}

function planPeakRateLabel(plan: SubscriptionPlan): string {
  return formatPeakRateWindow(plan, serverTimezoneLabel(appStore.cachedPublicSettings?.server_utc_offset))
}

function selectPlan(plan: SubscriptionPlan) {
  selectedPlan.value = plan
  errorMessage.value = ''
  errorHintMessage.value = ''
  closePlanGroupsModal()
}

function selectPlanFromModal(plan: SubscriptionPlan) {
  showRenewalModal.value = false
  renewGroupId.value = null
  selectedPlan.value = plan
  errorMessage.value = ''
  errorHintMessage.value = ''
}

function closeRenewalModal() {
  showRenewalModal.value = false
  renewGroupId.value = null
}

async function handleSubmitRecharge() {
  if (!canSubmit.value || submitting.value) return
  await createOrder(validAmount.value, 'balance')
}

async function confirmSubscribe() {
  const plan = selectedCheckoutPlan.value
  if (!plan || submitting.value) return
  selectedPlan.value = plan
  await createOrder(plan.price, 'subscription', plan.id)
}

async function createOrder(orderAmount: number, orderType: OrderType, planId?: number, options: CreateOrderOptions = {}) {
  submitting.value = true
  errorMessage.value = ''
  errorHintMessage.value = ''
  const requestType = normalizeVisibleMethod(options.paymentType || selectedMethod.value) || options.paymentType || selectedMethod.value
  try {
    const payload = buildCreateOrderPayload({
      amount: orderAmount,
      paymentType: requestType,
      orderType,
      planId,
      origin: typeof window !== 'undefined' ? window.location.origin : '',
      isMobile: isMobileDevice(),
      isWechatBrowser: typeof window !== 'undefined' && /MicroMessenger/i.test(window.navigator.userAgent),
      forceQRCode: !!(checkout.value.alipay_force_qrcode && normalizeVisibleMethod(requestType) === 'alipay'),
    })
    if (options.openid) {
      payload.openid = options.openid
    }
    if (options.wechatResumeToken) {
      payload.wechat_resume_token = options.wechatResumeToken
    }

    const result = await paymentStore.createOrder(payload) as CreateOrderResult & { resume_token?: string }
    const openWindow = (url: string) => {
      const win = window.open(url, 'paymentPopup', getPaymentPopupFeatures())
      if (!win || win.closed) {
        window.location.href = url
      }
    }
    const visibleMethod = normalizeVisibleMethod(requestType) || requestType
    // When user clicks the dedicated Stripe button, leave method blank so the
    // landing page renders Stripe's full Payment Element (card/link/alipay/wxpay).
    const stripeMethod = visibleMethod === 'stripe'
      ? ''
      : visibleMethod === 'wxpay' ? 'wechat_pay' : 'alipay'
    const stripeRouteUrl = result.client_secret && visibleMethod !== 'airwallex'
      ? router.resolve({
        path: '/payment/stripe',
        query: {
          order_id: String(result.order_id),
          client_secret: result.client_secret,
          method: stripeMethod || undefined,
          resume_token: result.resume_token || undefined,
        },
      }).href
      : ''
    const airwallexRouteUrl = result.client_secret && result.intent_id
      ? router.resolve({
        path: '/payment/airwallex',
        query: {
          order_id: String(result.order_id),
          out_trade_no: result.out_trade_no || undefined,
          resume_token: result.resume_token || undefined,
        },
      }).href
      : ''
    const decision = decidePaymentLaunch(result, {
      visibleMethod,
      orderType,
      isMobile: isMobileDevice(),
      isWechatBrowser: typeof window !== 'undefined' && /MicroMessenger/i.test(window.navigator.userAgent),
      forceQRCode: !!(checkout.value.alipay_force_qrcode && visibleMethod === 'alipay'),
      stripePopupUrl: stripeRouteUrl,
      stripeRouteUrl,
      airwallexRouteUrl,
    })

    if (decision.kind === 'wechat_oauth' && decision.oauth?.authorize_url) {
      window.location.href = buildWechatOAuthAuthorizeUrl(decision.oauth.authorize_url, {
        paymentType: visibleMethod,
        orderType,
        planId,
        orderAmount,
      })
      return
    }

    if (decision.kind === 'unhandled') {
      applyScenarioError({ reason: 'UNHANDLED_PAYMENT_SCENARIO' }, visibleMethod)
      return
    }

    paymentState.value = decision.paymentState
    paymentPhase.value = 'paying'
    persistRecoverySnapshot(decision.recovery)

    if (decision.kind === 'stripe_popup') {
      openWindow(decision.paymentState.payUrl)
      return
    }
    if (decision.kind === 'stripe_route') {
      window.location.href = decision.paymentState.payUrl
      return
    }
    if (decision.kind === 'airwallex_route') {
      window.location.href = decision.paymentState.payUrl
      return
    }
    if (decision.kind === 'wechat_jsapi' && decision.jsapi) {
      try {
        const jsapiResult = await invokeWechatJsapiPayment(decision.jsapi as Record<string, unknown>)
        const errMsg = String(jsapiResult.err_msg || '').toLowerCase()
        if (errMsg.includes('cancel')) {
          appStore.showInfo(t('payment.qr.cancelled'))
          resetPayment()
        } else if (errMsg && !errMsg.includes('ok')) {
          resetPayment()
          const fallbackApplied = await attemptMobileQrFallback(
            { reason: 'WECHAT_JSAPI_FAILED', message: errMsg },
            {
              orderAmount,
              orderType,
              planId,
              paymentType: visibleMethod,
              attempted: options.mobileQrFallbackAttempted === true,
            },
          )
          if (!fallbackApplied) {
            applyScenarioError({ reason: 'WECHAT_JSAPI_FAILED', message: errMsg }, visibleMethod)
          }
        } else {
          const resultState = { ...decision.paymentState }
          resetPayment()
          await redirectToPaymentResult(resultState)
        }
      } catch (err: unknown) {
        resetPayment()
        const fallbackApplied = await attemptMobileQrFallback(err, {
          orderAmount,
          orderType,
          planId,
          paymentType: visibleMethod,
          attempted: options.mobileQrFallbackAttempted === true,
        })
        if (!fallbackApplied) {
          throw err
        }
      }
      return
    }
    if (decision.kind === 'redirect_waiting' && decision.paymentState.payUrl) {
      if (isMobileDevice()) {
        window.location.href = decision.paymentState.payUrl
        return
      }
      openWindow(decision.paymentState.payUrl)
    }
  } catch (err: unknown) {
    const apiErr = err as Record<string, unknown>
    if (apiErr.reason === 'TOO_MANY_PENDING') {
      const metadata = apiErr.metadata as Record<string, unknown> | undefined
      errorMessage.value = t('payment.errors.tooManyPending', { max: metadata?.max || '' })
      errorHintMessage.value = ''
    } else if (apiErr.reason === 'CANCEL_RATE_LIMITED') {
      errorMessage.value = t('payment.errors.cancelRateLimited')
      errorHintMessage.value = ''
    } else if (await attemptMobileQrFallback(err, {
      orderAmount,
      orderType,
      planId,
      paymentType: requestType,
      attempted: options.mobileQrFallbackAttempted === true,
    })) {
      return
    } else {
      const handled = applyScenarioError(
        err,
        normalizeVisibleMethod(options.paymentType || selectedMethod.value) || selectedMethod.value,
      )
      if (!handled) {
        errorMessage.value = extractI18nErrorMessage(err, t, 'payment.errors', extractApiErrorMessage(err, t('payment.result.failed')))
        errorHintMessage.value = ''
      }
      if (handled) {
        return
      }
    }
    appStore.showError(buildPaymentErrorToastMessage(errorMessage.value, errorHintMessage.value))
  } finally {
    submitting.value = false
  }
}

interface MobileQrFallbackContext {
  orderAmount: number
  orderType: OrderType
  planId?: number
  paymentType: string
  attempted: boolean
}

function shouldFallbackToDesktopQr(err: unknown, paymentMethod: string, attempted: boolean): boolean {
  if (attempted || !isMobileDevice()) {
    return false
  }

  const normalizedMethod = normalizeVisibleMethod(paymentMethod) || paymentMethod
  const reason = typeof err === 'object' && err && 'reason' in err && typeof err.reason === 'string'
    ? err.reason
    : ''
  const message = err instanceof Error
    ? err.message
    : (typeof err === 'object' && err && 'message' in err && typeof err.message === 'string'
      ? err.message
      : '')
  const normalizedMessage = message.toLowerCase()

  if (normalizedMethod === 'wxpay') {
    return reason === 'WECHAT_H5_NOT_AUTHORIZED'
      || reason === 'WECHAT_PAYMENT_MP_NOT_CONFIGURED'
      || reason === 'WECHAT_JSAPI_FAILED'
      || reason === 'PAYMENT_GATEWAY_ERROR'
      || reason === 'UNHANDLED_PAYMENT_SCENARIO'
      || normalizedMessage.includes('weixinjsbridge is unavailable')
      || normalizedMessage.includes('wechat_jsapi_unavailable')
  }

  if (normalizedMethod === 'alipay') {
    return reason === 'PAYMENT_GATEWAY_ERROR' || reason === 'UNHANDLED_PAYMENT_SCENARIO'
  }

  return false
}

async function attemptMobileQrFallback(err: unknown, context: MobileQrFallbackContext): Promise<boolean> {
  if (!shouldFallbackToDesktopQr(err, context.paymentType, context.attempted)) {
    return false
  }

  try {
    const visibleMethod = normalizeVisibleMethod(context.paymentType) || context.paymentType
    const payload = buildCreateOrderPayload({
      amount: context.orderAmount,
      paymentType: visibleMethod,
      orderType: context.orderType,
      planId: context.planId,
      origin: typeof window !== 'undefined' ? window.location.origin : '',
      isMobile: false,
      isWechatBrowser: false,
    })
    const result = await paymentStore.createOrder(payload) as CreateOrderResult & { resume_token?: string }
    const stripeMethod = visibleMethod === 'wxpay' ? 'wechat_pay' : 'alipay'
    const stripeRouteUrl = result.client_secret
      ? router.resolve({
        path: '/payment/stripe',
        query: {
          order_id: String(result.order_id),
          client_secret: result.client_secret,
          method: stripeMethod,
          resume_token: result.resume_token || undefined,
        },
      }).href
      : ''
    const decision = decidePaymentLaunch(result, {
      visibleMethod,
      orderType: context.orderType,
      isMobile: false,
      isWechatBrowser: false,
      stripePopupUrl: stripeRouteUrl,
      stripeRouteUrl,
    })

    if (decision.kind !== 'qr_waiting' || !decision.paymentState.qrCode) {
      return false
    }

    errorMessage.value = ''
    errorHintMessage.value = ''
    paymentState.value = decision.paymentState
    paymentPhase.value = 'paying'
    persistRecoverySnapshot(decision.recovery)
    appStore.showWarning(t('payment.errors.mobilePaymentFallbackToQr'))
    return true
  } catch {
    return false
  }
}

function applyScenarioError(err: unknown, paymentMethod: string): boolean {
  const descriptor = describePaymentScenarioError(err, {
    paymentMethod,
    isMobile: isMobileDevice(),
    isWechatBrowser: typeof window !== 'undefined' && /MicroMessenger/i.test(window.navigator.userAgent),
  })
  if (!descriptor) {
    errorMessage.value = ''
    errorHintMessage.value = ''
    return false
  }
  errorMessage.value = t(descriptor.messageKey)
  errorHintMessage.value = descriptor.hintKey ? t(descriptor.hintKey) : ''
  appStore.showError(buildPaymentErrorToastMessage(errorMessage.value, errorHintMessage.value))
  return true
}

async function resumeWechatPaymentFromQuery() {
  const resume = parseWechatResumeRoute(route.query, checkout.value.plans, validAmount.value)
  if (!resume) {
    return
  }

  selectedMethod.value = resume.paymentType
  if (resume.orderType === 'balance' && resume.orderAmount > 0) {
    amount.value = resume.orderAmount
  }
  if (resume.orderType === 'subscription' && resume.planId) {
    selectedPlan.value = checkout.value.plans.find(plan => plan.id === resume.planId) ?? null
  }

  await router.replace({ path: route.path, query: stripWechatResumeQuery(route.query) })

  if (resume.wechatResumeToken) {
    await createOrder(0, resume.orderType, resume.planId, {
      wechatResumeToken: resume.wechatResumeToken,
      paymentType: resume.paymentType,
      isResume: true,
    })
    return
  }

  if (resume.orderAmount > 0 && resume.openid) {
    await createOrder(resume.orderAmount, resume.orderType, resume.planId, {
      openid: resume.openid,
      paymentType: resume.paymentType,
      isResume: true,
    })
  }
}



onMounted(async () => {
  try {
    const res = await paymentAPI.getCheckoutInfo()
    checkout.value = res.data
    if (enabledMethods.value.length) {
      const order: readonly string[] = METHOD_ORDER
      const sorted = [...enabledMethods.value].sort((a, b) => {
        const ai = order.indexOf(a)
        const bi = order.indexOf(b)
        return (ai === -1 ? 999 : ai) - (bi === -1 ? 999 : bi)
      })
      selectedMethod.value = sorted[0]
    }
    if (typeof window !== 'undefined') {
      if (hasWechatResumeQuery(route.query)) {
        removeRecoverySnapshot()
      }
      const routeResumeToken = typeof route.query.resume_token === 'string'
        ? route.query.resume_token
        : typeof route.query.wechat_resume_token === 'string'
          ? route.query.wechat_resume_token
          : undefined
      const restored = readPaymentRecoverySnapshot(
        window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY),
        { resumeToken: routeResumeToken },
      )
      if (restored) {
        paymentState.value = restored
        paymentPhase.value = 'paying'
        const restoredMethod = normalizeVisibleMethod(restored.paymentType)
          || (visibleMethods.value[restored.paymentType] ? restored.paymentType : '')
        if (restoredMethod) {
          selectedMethod.value = restoredMethod
        }
      } else {
        removeRecoverySnapshot()
      }
    }
    await resumeWechatPaymentFromQuery()
    if (checkout.value.balance_disabled) {
      activeTab.value = 'subscription'
    }
    // Handle renewal navigation: ?tab=subscription&group=123
    if (route.query.tab === 'subscription') {
      activeTab.value = 'subscription'
      if (route.query.group) {
        const groupId = Number(route.query.group)
        const groupPlans = checkout.value.plans.filter(p => p.group_id === groupId)
        if (groupPlans.length === 1) {
          selectedPlan.value = groupPlans[0]
        } else if (groupPlans.length > 1) {
          renewGroupId.value = groupId
          showRenewalModal.value = true
        }
      }
    }
  } catch (err: unknown) { appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error'))) }
  finally { loading.value = false }
  // Fetch active subscriptions (uses cache, non-blocking)
  subscriptionStore.fetchActiveSubscriptions().catch(() => {})
})
</script>

<style scoped>
.recommended-card {
  position: relative;
}

.recommended-card::before {
  content: '';
  position: absolute;
  inset: -1px;
  border-radius: inherit;
  padding: 1px;
  background: linear-gradient(135deg, rgba(20, 184, 166, 0.45), transparent, rgba(6, 182, 212, 0.45));
  -webkit-mask: linear-gradient(#fff 0 0) content-box, linear-gradient(#fff 0 0);
  -webkit-mask-composite: xor;
  mask-composite: exclude;
  pointer-events: none;
}

@keyframes shimmer-wave {
  0% { background-position: -200% 0; }
  100% { background-position: 200% 0; }
}

.btn-shimmer {
  background: linear-gradient(90deg, #14b8a6, #2dd4bf, #0f766e, #14b8a6);
  background-size: 400% 100%;
  animation: shimmer-wave 6s linear infinite;
  border: none;
}

.btn-shimmer:hover:not(:disabled) {
  background-size: 200% 100%;
  animation-duration: 3s;
  box-shadow: 0 0 25px rgba(20, 184, 166, 0.35);
}

.btn-shimmer:disabled {
  background: #1e293b;
  color: #64748b;
  animation: none;
  cursor: not-allowed;
}

.plan-description-clamp {
  display: -webkit-box;
  min-height: 1.95rem;
  overflow: hidden;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.group-scrollbar::-webkit-scrollbar {
  width: 4px;
}

.group-scrollbar::-webkit-scrollbar-track {
  background: transparent;
}

.group-scrollbar::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.08);
  border-radius: 99px;
}

.group-scrollbar::-webkit-scrollbar-thumb:hover {
  background: rgba(20, 184, 166, 0.3);
}
</style>
