<template>
  <div class="account-breadcrumb">
    <a href="/home">{{ t('accountPages.home') }}</a
    ><span>/</span><span>{{ title }}</span>
  </div>
  <header class="account-heading">
    <div>
      <p class="account-eyebrow">
        {{ section === 'subscriptions' ? 'MY SUBSCRIPTIONS' : 'MY ORDERS' }}
      </p>
      <h1>{{ title }}</h1>
      <p class="account-description">
        {{
          t(
            section === 'subscriptions'
              ? 'accountPages.subscriptionDescription'
              : 'accountPages.orderDescription',
          )
        }}
      </p>
    </div>
    <div class="account-heading-actions"><slot /></div>
  </header>
  <nav class="account-navigation" :aria-label="t('accountPages.subscriptions')">
    <RouterLink
      to="/subscriptions"
      :aria-current="section === 'subscriptions' ? 'page' : undefined"
      >{{ t('accountPages.subscriptions') }}</RouterLink
    ><RouterLink
      to="/orders"
      :aria-current="section === 'orders' ? 'page' : undefined"
      >{{ t('accountPages.orders') }}</RouterLink
    ><RouterLink to="/purchase"
      >{{ t('accountPages.purchase') }}<Icon name="arrowRight" size="sm"
    /></RouterLink>
  </nav>
</template>
<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
const props = defineProps<{ section: 'subscriptions' | 'orders' }>()
const { t } = useI18n()
const title = computed(() =>
  t(
    props.section === 'subscriptions'
      ? 'accountPages.subscriptions'
      : 'accountPages.orders',
  ),
)
</script>
