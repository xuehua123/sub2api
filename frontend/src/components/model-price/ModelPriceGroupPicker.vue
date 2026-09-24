<template>
  <section class="price-group-picker" aria-label="快捷选择分组">
    <div class="price-group-heading">
      <div>
        <h2>选择分组</h2>
        <p>
          {{ selected?.name || '选择一个分组查看价格'
          }}<span v-if="selected"> · {{ selected.model_count }} 个模型</span>
        </p>
      </div>
      <label class="price-group-search"
        ><Icon name="search" size="sm" /><input
          v-model="query"
          aria-label="搜索分组"
          placeholder="搜索分组名称"
          type="search"
      /></label>
    </div>
    <div class="price-group-platforms" aria-label="分组平台">
      <button type="button" :aria-pressed="!platform" @click="platform = ''">
        全部 <span>{{ groups.length }}</span></button
      ><button
        v-for="p in platforms"
        :key="p"
        type="button"
        :aria-pressed="platform === p"
        @click="platform = p"
      >
        {{ p }}
      </button>
    </div>
    <div class="price-group-options">
      <button
        v-for="g in visible"
        :key="g.id"
        type="button"
        :disabled="busy"
        :aria-pressed="selectedID === g.id"
        :data-testid="'quick-group-' + g.id"
        @click="$emit('select', g.id)"
      >
        <span class="price-group-name">{{ g.name }}</span
        ><span class="price-group-meta"
          >{{ g.platform }} · {{ g.model_count }} 模型{{
            g.hidden ? ' · 隐藏' : ''
          }}</span
        ><Icon v-if="selectedID === g.id" name="check" size="sm" />
      </button>
    </div>
    <p v-if="!visible.length" class="price-group-empty">
      没有匹配的分组<button
        v-if="query || platform"
        type="button"
        @click="reset"
      >
        清除筛选
      </button>
    </p>
  </section>
</template>
<script setup lang="ts">
import { computed, ref } from 'vue'
import type { ModelPriceGroup } from '@/api/modelPrices'
import Icon from '@/components/icons/Icon.vue'
const props = defineProps<{
  groups: ModelPriceGroup[]
  selectedID?: number
  busy: boolean
}>()
defineEmits<{ select: [id: number] }>()
const query = ref(''),
  platform = ref('')
const selected = computed(() =>
  props.groups.find((g) => g.id === props.selectedID),
)
const platforms = computed(() =>
  [...new Set(props.groups.map((g) => g.platform))].sort(),
)
const visible = computed(() =>
  props.groups.filter(
    (g) =>
      (!platform.value || g.platform === platform.value) &&
      (!query.value.trim() ||
        g.name.toLowerCase().includes(query.value.trim().toLowerCase())),
  ),
)
function reset() {
  query.value = ''
  platform.value = ''
}
</script>
