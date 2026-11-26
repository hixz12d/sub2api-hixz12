<template>
  <div class="space-y-5 border-t border-gray-100 pt-5 dark:border-dark-700">
    <!-- 1. Catalog configuration editor -->
    <div class="space-y-4">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div class="min-w-0">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('admin.settings.features.modelPlaza.catalog.title') }}
          </h3>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.settings.features.modelPlaza.catalog.description') }}
          </p>
        </div>

        <div class="flex shrink-0 items-center gap-2">
          <!-- Mode toggle: Visual vs JSON -->
          <div
            class="inline-flex rounded-lg border border-gray-200 bg-gray-50 p-0.5 dark:border-dark-600 dark:bg-dark-800"
            role="group"
          >
            <button
              type="button"
              :class="[
                'rounded-md px-2.5 py-1 text-xs font-medium transition-colors',
                mode === 'visual'
                  ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white'
                  : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200',
              ]"
              :aria-pressed="mode === 'visual'"
              @click="switchMode('visual')"
            >
              {{ t('admin.settings.features.modelPlaza.catalog.modeVisual') }}
            </button>
            <button
              type="button"
              :class="[
                'rounded-md px-2.5 py-1 text-xs font-medium transition-colors',
                mode === 'json'
                  ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white'
                  : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200',
              ]"
              :aria-pressed="mode === 'json'"
              @click="switchMode('json')"
            >
              {{ t('admin.settings.features.modelPlaza.catalog.modeJson') }}
            </button>
          </div>

          <button
            v-if="mode === 'visual'"
            type="button"
            class="btn btn-primary btn-sm shrink-0"
            @click="addGroupRow"
          >
            <Icon name="plus" size="sm" class="mr-1" />
            {{ t('admin.settings.features.modelPlaza.catalog.addGroup') }}
          </button>
        </div>
      </div>

      <!-- Visual mode -->
      <template v-if="mode === 'visual'">
        <div
          v-if="groupRows.length === 0"
          class="rounded-xl border border-dashed border-gray-300 px-4 py-3 text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400"
        >
          {{ t('admin.settings.features.modelPlaza.catalog.empty') }}
        </div>

        <div v-else class="space-y-3">
          <div
            v-for="(row, rowIdx) in groupRows"
            :key="row.uid"
            class="space-y-4 rounded-xl border border-gray-200 bg-gray-50/50 p-4 dark:border-dark-600 dark:bg-dark-800/40"
          >
            <!-- Row header: Group selector / ID + Enabled toggle + Remove -->
            <div class="flex flex-wrap items-end justify-between gap-3 border-b border-gray-200/80 pb-3 dark:border-dark-700">
              <div class="min-w-0 flex-1 space-y-1">
                <label class="block text-xs font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.settings.features.modelPlaza.catalog.groupId') }}
                </label>
                <div class="flex flex-wrap items-center gap-2">
                  <select
                    v-if="groups.length > 0"
                    :value="isKnownGroupId(row.groupId) ? row.groupId : ''"
                    class="input h-9 min-w-[200px] flex-1 py-1.5 text-sm"
                    @change="onGroupSelectChange(rowIdx, ($event.target as HTMLSelectElement).value)"
                  >
                    <option value="">
                      {{ t('admin.settings.features.modelPlaza.catalog.groupSelectPlaceholder') }}
                    </option>
                    <option
                      v-for="g in groups"
                      :key="g.id"
                      :value="String(g.id)"
                      :disabled="isGroupIdUsedByOtherRow(String(g.id), rowIdx)"
                    >
                      {{ formatGroupOptionLabel(g) }}
                    </option>
                  </select>
                  <input
                    :value="row.groupId"
                    type="text"
                    inputmode="numeric"
                    class="input h-9 w-36 py-1.5 font-mono text-sm"
                    :placeholder="t('admin.settings.features.modelPlaza.catalog.groupIdPlaceholder')"
                    @input="onGroupIdInput(rowIdx, ($event.target as HTMLInputElement).value)"
                  />
                </div>
                <p
                  v-if="row.groupId && duplicateGroupIds.has(row.groupId.trim())"
                  class="text-xs text-red-600 dark:text-red-400"
                >
                  {{ t('admin.settings.features.modelPlaza.catalog.duplicateGroupError') }}
                </p>
              </div>

              <div class="flex shrink-0 items-center gap-3">
                <label class="inline-flex items-center gap-2 text-xs font-medium text-gray-700 dark:text-gray-300">
                  <span>{{ t('admin.settings.features.modelPlaza.catalog.groupEnabled') }}</span>
                  <Toggle
                    :model-value="row.enabled"
                    @update:model-value="onRowFieldChange(rowIdx, 'enabled', $event)"
                  />
                </label>
                <button
                  type="button"
                  class="inline-flex items-center gap-1 rounded-lg px-2 py-1.5 text-xs text-gray-500 hover:bg-red-50 hover:text-red-600 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-red-400"
                  :title="t('admin.settings.features.modelPlaza.catalog.removeGroup')"
                  @click="removeGroupRow(rowIdx)"
                >
                  <Icon name="trash" size="sm" />
                  <span>{{ t('admin.settings.features.modelPlaza.catalog.removeGroup') }}</span>
                </button>
              </div>
            </div>

            <!-- Public showcase toggle + badge -->
            <div class="space-y-3">
              <div class="flex items-center justify-between gap-3">
                <div class="min-w-0">
                  <label class="text-xs font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.settings.features.modelPlaza.catalog.showcasePublic') }}
                  </label>
                  <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                    {{ t('admin.settings.features.modelPlaza.catalog.showcasePublicHint') }}
                  </p>
                </div>
                <Toggle
                  :model-value="row.showcasePublic"
                  @update:model-value="onRowFieldChange(rowIdx, 'showcasePublic', $event)"
                />
              </div>

              <div v-if="row.showcasePublic || row.showcaseBadge">
                <label class="block text-xs font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.settings.features.modelPlaza.catalog.showcaseBadge') }}
                </label>
                <input
                  :value="row.showcaseBadge"
                  type="text"
                  class="input mt-1 h-9 py-1.5 text-sm"
                  :placeholder="t('admin.settings.features.modelPlaza.catalog.showcaseBadgePlaceholder')"
                  @input="onRowFieldChange(rowIdx, 'showcaseBadge', ($event.target as HTMLInputElement).value)"
                />
              </div>
            </div>

            <!-- Models list -->
            <div>
              <label class="block text-xs font-medium text-gray-700 dark:text-gray-300">
                {{ t('admin.settings.features.modelPlaza.catalog.models') }}
              </label>
              <textarea
                :value="row.modelsText"
                rows="3"
                class="input mt-1 font-mono text-xs"
                :placeholder="t('admin.settings.features.modelPlaza.catalog.modelsPlaceholder')"
                @input="onRowFieldChange(rowIdx, 'modelsText', ($event.target as HTMLTextAreaElement).value)"
              ></textarea>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.settings.features.modelPlaza.catalog.modelsHint') }}
              </p>
            </div>

            <!-- Bare aliases -->
            <div>
              <label class="block text-xs font-medium text-gray-700 dark:text-gray-300">
                {{ t('admin.settings.features.modelPlaza.catalog.bareAliases') }}
              </label>
              <input
                :value="row.bareAliasesText"
                type="text"
                class="input mt-1 h-9 py-1.5 font-mono text-xs"
                :placeholder="t('admin.settings.features.modelPlaza.catalog.bareAliasesPlaceholder')"
                @input="onRowFieldChange(rowIdx, 'bareAliasesText', ($event.target as HTMLInputElement).value)"
              />
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.settings.features.modelPlaza.catalog.bareAliasesHint') }}
              </p>
            </div>

            <!-- Per-model notes -->
            <div class="space-y-2">
              <div class="flex items-center justify-between gap-2">
                <label class="text-xs font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.settings.features.modelPlaza.catalog.modelNotes') }}
                </label>
                <button
                  type="button"
                  class="btn btn-secondary btn-sm"
                  @click="addNoteRow(rowIdx)"
                >
                  <Icon name="plus" size="xs" class="mr-1" />
                  {{ t('admin.settings.features.modelPlaza.catalog.addNote') }}
                </button>
              </div>

              <div v-if="row.notes.length > 0" class="space-y-2">
                <div
                  v-for="(noteItem, noteIdx) in row.notes"
                  :key="noteIdx"
                  class="grid grid-cols-1 items-center gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,2fr)_2rem]"
                >
                  <input
                    :value="noteItem.model"
                    type="text"
                    class="input h-9 py-1.5 font-mono text-xs"
                    :placeholder="t('admin.settings.features.modelPlaza.catalog.noteModelPlaceholder')"
                    @input="onNoteChange(rowIdx, noteIdx, 'model', ($event.target as HTMLInputElement).value)"
                  />
                  <input
                    :value="noteItem.note"
                    type="text"
                    class="input h-9 py-1.5 text-xs"
                    :placeholder="t('admin.settings.features.modelPlaza.catalog.noteTextPlaceholder')"
                    @input="onNoteChange(rowIdx, noteIdx, 'note', ($event.target as HTMLInputElement).value)"
                  />
                  <button
                    type="button"
                    class="flex h-8 w-8 items-center justify-center rounded-lg text-gray-400 hover:bg-red-50 hover:text-red-600 dark:hover:bg-dark-700 dark:hover:text-red-400"
                    @click="removeNoteRow(rowIdx, noteIdx)"
                  >
                    <Icon name="x" size="sm" />
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </template>

      <!-- JSON source mode -->
      <div v-else class="space-y-2">
        <div class="flex items-center justify-between gap-2">
          <label class="text-xs font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.settings.features.modelPlaza.catalog.jsonLabel') }}
          </label>
          <button
            type="button"
            class="btn btn-secondary btn-sm"
            @click="formatJsonSource"
          >
            {{ t('admin.settings.features.modelPlaza.catalog.jsonFormat') }}
          </button>
        </div>
        <textarea
          :value="jsonText"
          rows="12"
          class="input font-mono text-xs leading-relaxed"
          spellcheck="false"
          @input="onJsonInput(($event.target as HTMLTextAreaElement).value)"
        ></textarea>
        <p v-if="jsonError" class="text-xs text-red-600 dark:text-red-400">
          {{ jsonError }}
        </p>
        <p class="text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.settings.features.modelPlaza.catalog.jsonHint') }}
        </p>
      </div>
    </div>

    <!-- 2. Availability Probe API Key IDs -->
    <div class="border-t border-gray-100 pt-5 dark:border-dark-700">
      <label class="block text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t('admin.settings.features.modelPlaza.probeKeys.title') }}
      </label>
      <p class="mb-2 mt-0.5 text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.settings.features.modelPlaza.probeKeys.hint') }}
      </p>
      <input
        :value="probeKeysInput"
        type="text"
        class="input font-mono text-sm"
        :placeholder="t('admin.settings.features.modelPlaza.probeKeys.placeholder')"
        @input="onProbeKeysInput(($event.target as HTMLInputElement).value)"
      />
      <p v-if="probeKeysError" class="mt-1 text-xs text-red-600 dark:text-red-400">
        {{ probeKeysError }}
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import Toggle from '@/components/common/Toggle.vue'
import type {
  ModelPlazaCatalogConfig,
  ModelPlazaCatalogGroupConfig
} from '@/api/admin/settings'
import type { AdminGroup } from '@/types'

const props = withDefaults(
  defineProps<{
    catalogConfig?: ModelPlazaCatalogConfig | null
    probeApiKeyIds?: number[] | null
    groups?: AdminGroup[]
  }>(),
  {
    catalogConfig: () => ({ groups: {} }),
    probeApiKeyIds: () => [],
    groups: () => []
  }
)

const emit = defineEmits<{
  (e: 'update:catalogConfig', value: ModelPlazaCatalogConfig): void
  (e: 'update:probeApiKeyIds', value: number[]): void
}>()

const { t } = useI18n()

interface NoteRow {
  model: string
  note: string
}

interface GroupRow {
  uid: number
  groupId: string
  enabled: boolean
  showcasePublic: boolean
  showcaseBadge: string
  modelsText: string
  bareAliasesText: string
  notes: NoteRow[]
}

let nextUid = 1
const mode = ref<'visual' | 'json'>('visual')
const groupRows = ref<GroupRow[]>([])
const jsonText = ref('{\n  "groups": {}\n}')
const jsonError = ref('')

const probeKeysInput = ref('')
const probeKeysError = ref('')

let lastEmittedCatalogJson = ''
let lastEmittedProbeKey = ''

function normalizeCatalogConfig(input?: ModelPlazaCatalogConfig | null): ModelPlazaCatalogConfig {
  const groups: Record<string, ModelPlazaCatalogGroupConfig> = {}
  const rawGroups = input?.groups
  if (rawGroups && typeof rawGroups === 'object' && !Array.isArray(rawGroups)) {
    for (const [rawId, cfg] of Object.entries(rawGroups)) {
      const id = rawId.trim()
      if (!id || !cfg || typeof cfg !== 'object') continue
      const models = Array.isArray(cfg.models)
        ? cfg.models.map((m) => String(m).trim()).filter(Boolean)
        : []
      const bareAliases = Array.isArray(cfg.bare_aliases)
        ? cfg.bare_aliases.map((m) => String(m).trim()).filter(Boolean)
        : []
      const modelNotes: Record<string, string> = {}
      if (cfg.model_notes && typeof cfg.model_notes === 'object' && !Array.isArray(cfg.model_notes)) {
        for (const [modelKey, noteVal] of Object.entries(cfg.model_notes)) {
          const mk = modelKey.trim()
          const nv = String(noteVal ?? '').trim()
          if (mk && nv) {
            modelNotes[mk] = nv
          }
        }
      }
      groups[id] = {
        enabled: cfg.enabled !== false,
        showcase_public: Boolean(cfg.showcase_public),
        showcase_badge: String(cfg.showcase_badge ?? '').trim(),
        models,
        bare_aliases: bareAliases,
        model_notes: modelNotes
      }
    }
  }
  return { groups }
}

function catalogToRows(config: ModelPlazaCatalogConfig): GroupRow[] {
  const normalized = normalizeCatalogConfig(config)
  return Object.entries(normalized.groups).map(([groupId, cfg]) => ({
    uid: nextUid++,
    groupId,
    enabled: cfg.enabled,
    showcasePublic: Boolean(cfg.showcase_public),
    showcaseBadge: cfg.showcase_badge ?? '',
    modelsText: (cfg.models ?? []).join('\n'),
    bareAliasesText: (cfg.bare_aliases ?? []).join(', '),
    notes: Object.entries(cfg.model_notes ?? {}).map(([model, note]) => ({ model, note }))
  }))
}

function parseListInput(raw: string): string[] {
  const seen = new Set<string>()
  const result: string[] = []
  for (const token of raw.split(/[\r\n,]+/)) {
    const item = token.trim()
    if (!item || seen.has(item)) continue
    seen.add(item)
    result.push(item)
  }
  return result
}

function rowsToCatalog(rows: GroupRow[]): ModelPlazaCatalogConfig {
  const groups: Record<string, ModelPlazaCatalogGroupConfig> = {}
  for (const row of rows) {
    const id = row.groupId.trim()
    if (!id || groups[id]) continue
    const modelNotes: Record<string, string> = {}
    for (const item of row.notes) {
      const mk = item.model.trim()
      const nv = item.note.trim()
      if (mk && nv) {
        modelNotes[mk] = nv
      }
    }
    groups[id] = {
      enabled: row.enabled,
      showcase_public: row.showcasePublic,
      showcase_badge: row.showcaseBadge.trim(),
      models: parseListInput(row.modelsText),
      bare_aliases: parseListInput(row.bareAliasesText),
      model_notes: modelNotes
    }
  }
  return { groups }
}

function syncFromExternalCatalog(next?: ModelPlazaCatalogConfig | null) {
  const normalized = normalizeCatalogConfig(next)
  const serialized = JSON.stringify(normalized, null, 2)
  if (serialized === lastEmittedCatalogJson) return
  lastEmittedCatalogJson = serialized
  groupRows.value = catalogToRows(normalized)
  jsonText.value = serialized
  jsonError.value = ''
}

watch(
  () => props.catalogConfig,
  (val) => {
    syncFromExternalCatalog(val)
  },
  { immediate: true, deep: true }
)

const duplicateGroupIds = computed<Set<string>>(() => {
  const counts = new Map<string, number>()
  for (const row of groupRows.value) {
    const id = row.groupId.trim()
    if (!id) continue
    counts.set(id, (counts.get(id) ?? 0) + 1)
  }
  const dups = new Set<string>()
  for (const [id, count] of counts.entries()) {
    if (count > 1) dups.add(id)
  }
  return dups
})

function isKnownGroupId(groupId: string): boolean {
  const id = groupId.trim()
  if (!id) return false
  return props.groups.some((g) => String(g.id) === id)
}

function isGroupIdUsedByOtherRow(candidateId: string, currentRowIdx: number): boolean {
  return groupRows.value.some(
    (row, idx) => idx !== currentRowIdx && row.groupId.trim() === candidateId
  )
}

function formatGroupOptionLabel(group: AdminGroup): string {
  return `${group.name} (${group.id})`
}

function emitFromVisualRows() {
  const catalog = rowsToCatalog(groupRows.value)
  const serialized = JSON.stringify(catalog, null, 2)
  lastEmittedCatalogJson = serialized
  jsonText.value = serialized
  jsonError.value = ''
  emit('update:catalogConfig', catalog)
}

function addGroupRow() {
  groupRows.value = [
    ...groupRows.value,
    {
      uid: nextUid++,
      groupId: '',
      enabled: true,
      showcasePublic: false,
      showcaseBadge: '',
      modelsText: '',
      bareAliasesText: '',
      notes: []
    }
  ]
}

function removeGroupRow(index: number) {
  const next = [...groupRows.value]
  next.splice(index, 1)
  groupRows.value = next
  emitFromVisualRows()
}

function onGroupSelectChange(index: number, value: string) {
  const row = groupRows.value[index]
  if (!row) return
  row.groupId = value.trim()
  emitFromVisualRows()
}

function onGroupIdInput(index: number, value: string) {
  const row = groupRows.value[index]
  if (!row) return
  row.groupId = value.replace(/[^\d]/g, '')
  emitFromVisualRows()
}

function onRowFieldChange<K extends keyof GroupRow>(index: number, field: K, value: GroupRow[K]) {
  const row = groupRows.value[index]
  if (!row) return
  row[field] = value
  emitFromVisualRows()
}

function addNoteRow(rowIdx: number) {
  const row = groupRows.value[rowIdx]
  if (!row) return
  row.notes = [...row.notes, { model: '', note: '' }]
}

function removeNoteRow(rowIdx: number, noteIdx: number) {
  const row = groupRows.value[rowIdx]
  if (!row) return
  const next = [...row.notes]
  next.splice(noteIdx, 1)
  row.notes = next
  emitFromVisualRows()
}

function onNoteChange(rowIdx: number, noteIdx: number, field: 'model' | 'note', value: string) {
  const row = groupRows.value[rowIdx]
  const note = row?.notes[noteIdx]
  if (!note) return
  note[field] = value
  emitFromVisualRows()
}

function parseAndValidateCatalogJson(raw: string): ModelPlazaCatalogConfig | null {
  const trimmed = raw.trim()
  if (!trimmed) {
    return { groups: {} }
  }
  let parsed: unknown
  try {
    parsed = JSON.parse(trimmed)
  } catch {
    return null
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    return null
  }
  const obj = parsed as Record<string, unknown>
  if (!('groups' in obj)) {
    return null
  }
  const groupsVal = obj.groups
  if (!groupsVal || typeof groupsVal !== 'object' || Array.isArray(groupsVal)) {
    return null
  }
  for (const [key, entry] of Object.entries(groupsVal as Record<string, unknown>)) {
    if (!key.trim() || !entry || typeof entry !== 'object' || Array.isArray(entry)) {
      return null
    }
    const g = entry as Record<string, unknown>
    if (g.enabled !== undefined && typeof g.enabled !== 'boolean') return null
    if (g.showcase_public !== undefined && typeof g.showcase_public !== 'boolean') return null
    if (g.showcase_badge !== undefined && typeof g.showcase_badge !== 'string') return null
    if (g.models !== undefined && !Array.isArray(g.models)) return null
    if (g.bare_aliases !== undefined && !Array.isArray(g.bare_aliases)) return null
    if (
      g.model_notes !== undefined &&
      (!g.model_notes || typeof g.model_notes !== 'object' || Array.isArray(g.model_notes))
    ) {
      return null
    }
  }
  return normalizeCatalogConfig(parsed as ModelPlazaCatalogConfig)
}

function onJsonInput(raw: string) {
  jsonText.value = raw
  const parsed = parseAndValidateCatalogJson(raw)
  if (!parsed) {
    jsonError.value = t('admin.settings.features.modelPlaza.catalog.jsonInvalid')
    return
  }
  jsonError.value = ''
  const serialized = JSON.stringify(parsed, null, 2)
  lastEmittedCatalogJson = serialized
  groupRows.value = catalogToRows(parsed)
  emit('update:catalogConfig', parsed)
}

function formatJsonSource() {
  const parsed = parseAndValidateCatalogJson(jsonText.value)
  if (!parsed) {
    jsonError.value = t('admin.settings.features.modelPlaza.catalog.jsonInvalid')
    return
  }
  jsonError.value = ''
  const serialized = JSON.stringify(parsed, null, 2)
  jsonText.value = serialized
  lastEmittedCatalogJson = serialized
  groupRows.value = catalogToRows(parsed)
  emit('update:catalogConfig', parsed)
}

function switchMode(nextMode: 'visual' | 'json') {
  if (nextMode === mode.value) return
  if (mode.value === 'json') {
    const parsed = parseAndValidateCatalogJson(jsonText.value)
    if (!parsed) {
      jsonError.value = t('admin.settings.features.modelPlaza.catalog.jsonInvalid')
      return
    }
    groupRows.value = catalogToRows(parsed)
    jsonError.value = ''
  } else {
    const catalog = rowsToCatalog(groupRows.value)
    jsonText.value = JSON.stringify(catalog, null, 2)
    jsonError.value = ''
  }
  mode.value = nextMode
}

// Probe API Key IDs synchronization & validation
function normalizeProbeKeyIds(ids?: number[] | null): number[] {
  if (!Array.isArray(ids)) return []
  const seen = new Set<number>()
  const result: number[] = []
  for (const id of ids) {
    const n = Number(id)
    if (Number.isSafeInteger(n) && n > 0 && !seen.has(n)) {
      seen.add(n)
      result.push(n)
    }
  }
  return result
}

watch(
  () => props.probeApiKeyIds,
  (ids) => {
    const normalized = normalizeProbeKeyIds(ids)
    const key = normalized.join(',')
    if (key === lastEmittedProbeKey) return
    lastEmittedProbeKey = key
    probeKeysInput.value = normalized.join(', ')
    probeKeysError.value = ''
  },
  { immediate: true, deep: true }
)

function onProbeKeysInput(raw: string) {
  probeKeysInput.value = raw
  const trimmed = raw.trim()
  if (!trimmed) {
    probeKeysError.value = ''
    lastEmittedProbeKey = ''
    emit('update:probeApiKeyIds', [])
    return
  }
  const tokens = trimmed.split(/[\s,，]+/).filter(Boolean)
  const seen = new Set<number>()
  const ids: number[] = []
  for (const token of tokens) {
    if (!/^\d+$/.test(token)) {
      probeKeysError.value = t('admin.settings.features.modelPlaza.probeKeys.invalid')
      return
    }
    const num = Number(token)
    if (!Number.isSafeInteger(num) || num <= 0) {
      probeKeysError.value = t('admin.settings.features.modelPlaza.probeKeys.invalid')
      return
    }
    if (!seen.has(num)) {
      seen.add(num)
      ids.push(num)
    }
  }
  probeKeysError.value = ''
  lastEmittedProbeKey = ids.join(',')
  emit('update:probeApiKeyIds', ids)
}

function validate(): { valid: boolean; errorMessage?: string } {
  if (mode.value === 'json') {
    const parsed = parseAndValidateCatalogJson(jsonText.value)
    if (!parsed) {
      const msg = t('admin.settings.features.modelPlaza.catalog.jsonInvalid')
      jsonError.value = msg
      return { valid: false, errorMessage: msg }
    }
  }
  if (duplicateGroupIds.value.size > 0) {
    return {
      valid: false,
      errorMessage: t('admin.settings.features.modelPlaza.catalog.duplicateGroupError')
    }
  }
  if (probeKeysError.value) {
    return { valid: false, errorMessage: probeKeysError.value }
  }
  return { valid: true }
}

defineExpose({ validate })
</script>
