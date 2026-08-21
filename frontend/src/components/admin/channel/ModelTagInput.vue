<template>
  <div class="relative">
    <!-- Tags display -->
    <div class="flex flex-wrap gap-1.5 rounded-lg border border-gray-200 bg-white p-2 dark:border-dark-600 dark:bg-dark-800 min-h-[2.5rem]">
      <span
        v-for="(model, idx) in models"
        :key="idx"
        class="inline-flex items-center gap-1 rounded-md px-2 py-0.5 text-sm"
        :class="getPlatformTagClass(props.platform || '')"
      >
        {{ model }}
        <button
          type="button"
          @click="removeModel(idx)"
          class="ml-0.5 rounded-full p-0.5 hover:bg-primary-200 dark:hover:bg-primary-800"
        >
          <Icon name="x" size="xs" />
        </button>
      </span>
      <input
        ref="inputRef"
        v-model="inputValue"
        type="text"
        class="flex-1 min-w-[120px] border-none bg-transparent text-sm outline-none placeholder:text-gray-400 dark:text-white"
        :placeholder="models.length === 0 ? placeholder : ''"
        @keydown.enter.prevent="addModel"
        @keydown.tab.prevent="addModel"
        @keydown.delete="handleBackspace"
        @paste="handlePaste"
        @focus="focused = true"
        @blur="onBlur"
      />
    </div>
    <div
      v-if="showSuggestions"
      class="absolute left-0 right-0 z-20 mt-1 max-h-48 overflow-auto rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-600 dark:bg-dark-700"
    >
      <button
        v-for="item in filteredSuggestions"
        :key="item"
        type="button"
        class="block w-full px-3 py-1.5 text-left text-sm text-gray-900 hover:bg-gray-100 dark:text-gray-100 dark:hover:bg-dark-600"
        @mousedown.prevent="pickSuggestion(item)"
      >
        {{ item }}
      </button>
    </div>
    <p class="mt-1 text-xs text-gray-400">
      {{ t('admin.channels.form.modelInputHint', 'Press Enter to add, supports paste for batch import.') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { getPlatformTagClass } from './types'

const { t } = useI18n()

const props = defineProps<{
  models: string[]
  placeholder?: string
  platform?: string
  suggestions?: string[]
}>()

const emit = defineEmits<{
  'update:models': [models: string[]]
}>()

const inputValue = ref('')
const inputRef = ref<HTMLInputElement>()
const focused = ref(false)

const filteredSuggestions = computed(() => {
  const list = props.suggestions ?? []
  const q = inputValue.value.trim().toLowerCase()
  return list
    .filter((m) => !props.models.includes(m) && (!q || m.toLowerCase().includes(q)))
    .slice(0, 20)
})

const showSuggestions = computed(() => focused.value && filteredSuggestions.value.length > 0)

function addModel() {
  const val = inputValue.value.trim()
  if (!val) return
  if (!props.models.includes(val)) {
    emit('update:models', [...props.models, val])
  }
  inputValue.value = ''
}

function pickSuggestion(model: string) {
  if (!props.models.includes(model)) {
    emit('update:models', [...props.models, model])
  }
  inputValue.value = ''
  focused.value = true
  inputRef.value?.focus()
}

function removeModel(idx: number) {
  const newModels = [...props.models]
  newModels.splice(idx, 1)
  emit('update:models', newModels)
}

function handleBackspace() {
  if (inputValue.value === '' && props.models.length > 0) {
    removeModel(props.models.length - 1)
  }
}

function onBlur() {
  addModel()
  focused.value = false
}

function handlePaste(e: ClipboardEvent) {
  e.preventDefault()
  const text = e.clipboardData?.getData('text') || ''
  const items = text.split(/[,\n;]+/).map(s => s.trim()).filter(Boolean)
  if (items.length === 0) return
  const unique = [...new Set([...props.models, ...items])]
  emit('update:models', unique)
  inputValue.value = ''
}
</script>
