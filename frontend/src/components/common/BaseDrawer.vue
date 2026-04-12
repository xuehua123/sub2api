<template>
  <Teleport to="body">
    <!-- Backdrop overlay -->
    <Transition name="drawer-fade">
      <div
        v-if="show"
        class="fixed inset-0 bg-black/50 z-40 transition-opacity"
        :style="backdropZIndex"
        @click.self="handleClose"
        aria-hidden="true"
      ></div>
    </Transition>

    <!-- Drawer panel -->
    <Transition :name="`drawer-slide-${placement}`">
      <div
        v-if="show"
        class="fixed bg-white dark:bg-dark-800 shadow-xl flex flex-col z-50 overflow-hidden"
        :class="[
          placement === 'right' ? 'top-0 right-0 bottom-0' : '',
          placement === 'left' ? 'top-0 left-0 bottom-0' : '',
          placement === 'top' ? 'top-0 left-0 right-0' : '',
          placement === 'bottom' ? 'bottom-0 left-0 right-0' : '',
          drawerSizeClass
        ]"
        :style="drawerZIndex"
        role="dialog"
        :aria-labelledby="drawerId"
        aria-modal="true"
        ref="drawerRef"
      >
        <!-- Header -->
        <div class="flex items-center justify-between px-6 py-4 border-b border-gray-200 dark:border-dark-700">
          <h3 :id="drawerId" class="text-lg font-medium text-gray-900 dark:text-dark-50">
            {{ title }}
          </h3>
          <button
            @click="emit('close')"
            class="text-gray-400 hover:text-gray-500 hover:bg-gray-100 dark:hover:bg-dark-700 dark:hover:text-dark-300 rounded-lg p-1.5 focus:outline-none transition-colors"
            aria-label="Close drawer"
          >
            <Icon name="x" size="md" />
          </button>
        </div>

        <!-- Body -->
        <div class="flex-1 overflow-y-auto px-6 py-4">
          <slot></slot>
        </div>

        <!-- Footer -->
        <div v-if="$slots.footer" class="px-6 py-4 border-t border-gray-200 dark:border-dark-700 bg-gray-50 dark:bg-dark-900/50">
          <slot name="footer"></slot>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, watch, onMounted, onUnmounted, ref, nextTick } from 'vue'
import Icon from '@/components/icons/Icon.vue'

let drawerIdCounter = 0
const drawerId = `drawer-title-${++drawerIdCounter}`

const drawerRef = ref<HTMLElement | null>(null)
let previousActiveElement: HTMLElement | null = null

type Placement = 'left' | 'right' | 'top' | 'bottom'
type Size = 'sm' | 'md' | 'lg' | 'xl' | 'full'

interface OmitZIndexProps {
  show: boolean
  title: string
  placement?: Placement
  size?: Size
  closeOnEscape?: boolean
  closeOnClickOutside?: boolean
  zIndex?: number
}

const props = withDefaults(defineProps<OmitZIndexProps>(), {
  placement: 'right',
  size: 'md',
  closeOnEscape: true,
  closeOnClickOutside: true,
  zIndex: 40
})

const emit = defineEmits<{
  (e: 'close'): void
}>()

const backdropZIndex = computed(() => ({
  zIndex: props.zIndex
}))

const drawerZIndex = computed(() => ({
  zIndex: props.zIndex + 10
}))

const drawerSizeClass = computed(() => {
  if (props.placement === 'left' || props.placement === 'right') {
    const sizes: Record<Size, string> = {
      sm: 'w-64 max-w-full',
      md: 'w-80 max-w-full',
      lg: 'w-96 max-w-full',
      xl: 'w-120 max-w-full', // might need w-[30rem] or custom class
      full: 'w-full'
    }
    // Tailwind normally supports w-96, but for xl let's use custom class w-[32rem]
    if (props.size === 'xl') return 'w-[32rem] max-w-[100vw]'
    if (props.size === 'full') return 'w-[100vw] max-w-[100vw]'
    return sizes[props.size] || 'w-80 max-w-full'
  } else {
    // top or bottom
    const heights: Record<Size, string> = {
      sm: 'h-1/4',
      md: 'h-1/3',
      lg: 'h-1/2',
      xl: 'h-2/3',
      full: 'h-full'
    }
    return heights[props.size] || 'h-1/2'
  }
})

const handleClose = () => {
  if (props.closeOnClickOutside) {
    emit('close')
  }
}

const handleEscape = (event: KeyboardEvent) => {
  if (props.show && props.closeOnEscape && event.key === 'Escape') {
    emit('close')
  }
}

watch(
  () => props.show,
  async (isOpen) => {
    if (isOpen) {
      previousActiveElement = document.activeElement as HTMLElement
      document.body.classList.add('overflow-hidden') // stop body scrolling

      await nextTick()
      if (drawerRef.value) {
        const firstFocusable = drawerRef.value.querySelector<HTMLElement>(
          'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
        )
        firstFocusable?.focus()
      }
    } else {
      document.body.classList.remove('overflow-hidden')
      if (previousActiveElement && typeof previousActiveElement.focus === 'function') {
        previousActiveElement.focus()
      }
      previousActiveElement = null
    }
  },
  { immediate: true }
)

onMounted(() => {
  document.addEventListener('keydown', handleEscape)
})

onUnmounted(() => {
  document.removeEventListener('keydown', handleEscape)
  document.body.classList.remove('overflow-hidden')
})
</script>

<style>
/* Backdrop transition */
.drawer-fade-enter-active,
.drawer-fade-leave-active {
  transition: opacity 0.3s ease;
}
.drawer-fade-enter-from,
.drawer-fade-leave-to {
  opacity: 0;
}

/* Slide Right */
.drawer-slide-right-enter-active,
.drawer-slide-right-leave-active {
  transition: transform 0.3s ease-in-out;
}
.drawer-slide-right-enter-from,
.drawer-slide-right-leave-to {
  transform: translateX(100%);
}

/* Slide Left */
.drawer-slide-left-enter-active,
.drawer-slide-left-leave-active {
  transition: transform 0.3s ease-in-out;
}
.drawer-slide-left-enter-from,
.drawer-slide-left-leave-to {
  transform: translateX(-100%);
}

/* Slide Top */
.drawer-slide-top-enter-active,
.drawer-slide-top-leave-active {
  transition: transform 0.3s ease-in-out;
}
.drawer-slide-top-enter-from,
.drawer-slide-top-leave-to {
  transform: translateY(-100%);
}

/* Slide Bottom */
.drawer-slide-bottom-enter-active,
.drawer-slide-bottom-leave-active {
  transition: transform 0.3s ease-in-out;
}
.drawer-slide-bottom-enter-from,
.drawer-slide-bottom-leave-to {
  transform: translateY(100%);
}
</style>
