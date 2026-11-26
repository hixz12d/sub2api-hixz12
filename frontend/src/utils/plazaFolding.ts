/**
 * Pure thinking-effort variant folding utilities for Model Plaza lists and detail drawer.
 *
 * Rules:
 * - Recognizes effort suffixes: -minimal, -low, -medium, -high, -xhigh, -max.
 * - Folds models into a single entry ONLY when a family has >= 2 distinct suffixed variants
 *   within the same platform.
 * - Default selected effort always prefers `high`; otherwise falls back to the first variant
 *   in ascending effort order.
 * - Deduplicates bare family aliases (e.g. `gemini-4-neon` alongside `gemini-4-neon-{low,medium,high,xhigh}`)
 *   and tracks `bareAliasForHigh` when either the bare alias is present or a variant carries
 *   `bare_alias_for_high === true`.
 */

import type { PlazaModel } from '@/api/modelPlaza'

export type PlazaEffortLevel = 'minimal' | 'low' | 'medium' | 'high' | 'xhigh' | 'max'

export const PLAZA_EFFORT_ORDER: readonly PlazaEffortLevel[] = [
  'minimal',
  'low',
  'medium',
  'high',
  'xhigh',
  'max'
] as const

const EFFORT_RANK: Readonly<Record<PlazaEffortLevel, number>> = {
  minimal: 0,
  low: 1,
  medium: 2,
  high: 3,
  xhigh: 4,
  max: 5
}

const EFFORT_SUFFIX_RE = /^(.+)-(minimal|low|medium|high|xhigh|max)$/i

export interface ParsedEffortModelName {
  baseName: string
  effort: PlazaEffortLevel
}

export interface PlazaEffortVariant {
  effort: PlazaEffortLevel
  label: string
  model: PlazaModel
}

export interface PlazaFoldedEntry {
  /** Stable identifier (`${platform}:${baseName}`). */
  key: string
  /** Family base name when folded (e.g. "gemini-4-neon"), or model.name when standalone. */
  baseName: string
  platform: string
  /** True when >= 2 effort variants were folded together. */
  isFolded: boolean
  /** Sorted effort variants (length >= 2 when isFolded is true, empty when standalone). */
  variants: PlazaEffortVariant[]
  /** Default effort level (prefers 'high', else first variant), null when standalone. */
  defaultEffort: PlazaEffortLevel | null
  /** Default model to display (prefers 'high' variant when folded, or the standalone model). */
  defaultModel: PlazaModel
  /** All underlying models represented by this entry. */
  models: PlazaModel[]
  /** True if the bare family name aliases to the -high variant. */
  bareAliasForHigh: boolean
  /** Bare alias model ID (equals baseName) when bareAliasForHigh is true, else null. */
  bareAliasName: string | null
}

/**
 * Parses a trailing effort suffix from a model ID.
 * Returns null if the model ID does not end with a known effort tier suffix.
 */
export function parseEffortSuffix(modelName: string): ParsedEffortModelName | null {
  const trimmed = (modelName || '').trim()
  if (!trimmed) return null
  const match = EFFORT_SUFFIX_RE.exec(trimmed)
  if (!match) return null
  const baseName = match[1].trim()
  if (!baseName) return null
  const effort = match[2].toLowerCase() as PlazaEffortLevel
  return { baseName, effort }
}

function familyBucketKey(platform: string, baseName: string): string {
  return `${(platform || '').trim().toLowerCase()}::${baseName.toLowerCase()}`
}

function selectDefaultVariant(variants: readonly PlazaEffortVariant[]): PlazaEffortVariant {
  return variants.find((v) => v.effort === 'high') ?? variants[0]
}

/**
 * Folds models sharing the same platform and family base name when >= 2 distinct effort
 * suffixes exist, preserving first-appearance order and deduplicating bare aliases.
 */
export function foldPlazaModels(models: readonly PlazaModel[]): PlazaFoldedEntry[] {
  interface FamilyAccumulator {
    baseName: string
    platform: string
    firstIndex: number
    variantsByEffort: Map<PlazaEffortLevel, PlazaModel>
    bareModel: PlazaModel | null
  }

  const families = new Map<string, FamilyAccumulator>()
  const bareCandidates = new Map<string, { index: number; model: PlazaModel }>()

  // Pass 1: collect all suffixed variants and track potential bare-name candidates.
  models.forEach((model, index) => {
    const parsed = parseEffortSuffix(model.name)
    if (parsed) {
      const key = familyBucketKey(model.platform, parsed.baseName)
      let acc = families.get(key)
      if (!acc) {
        acc = {
          baseName: parsed.baseName,
          platform: model.platform,
          firstIndex: index,
          variantsByEffort: new Map<PlazaEffortLevel, PlazaModel>(),
          bareModel: null
        }
        families.set(key, acc)
      }
      if (!acc.variantsByEffort.has(parsed.effort)) {
        acc.variantsByEffort.set(parsed.effort, model)
      }
      return
    }

    const bareKey = familyBucketKey(model.platform, model.name.trim())
    if (!bareCandidates.has(bareKey)) {
      bareCandidates.set(bareKey, { index, model })
    }
  })

  // Pass 2: determine active folded families (>= 2 distinct effort tiers) and bind bare aliases.
  const activeFoldedFamilies = new Map<string, FamilyAccumulator>()
  const deduplicatedBareModels = new Set<PlazaModel>()

  for (const [key, acc] of families.entries()) {
    if (acc.variantsByEffort.size < 2) continue
    const bare = bareCandidates.get(key)
    if (bare) {
      acc.bareModel = bare.model
      acc.firstIndex = Math.min(acc.firstIndex, bare.index)
      deduplicatedBareModels.add(bare.model)
    }
    activeFoldedFamilies.set(key, acc)
  }

  // Pass 3: emit folded and standalone entries in stable first-appearance order.
  const entries: PlazaFoldedEntry[] = []
  const emittedFamilies = new Set<string>()

  const buildFoldedEntry = (acc: FamilyAccumulator): PlazaFoldedEntry => {
    const variants: PlazaEffortVariant[] = [...acc.variantsByEffort.entries()]
      .sort(([a], [b]) => EFFORT_RANK[a] - EFFORT_RANK[b])
      .map(([effort, model]) => ({
        effort,
        label: effort,
        model
      }))

    const defaultVariant = selectDefaultVariant(variants)
    const hasHighVariant = variants.some((v) => v.effort === 'high')
    const bareAliasForHigh =
      (acc.bareModel != null && hasHighVariant) ||
      Boolean(acc.bareModel?.bare_alias_for_high) ||
      variants.some((v) => Boolean(v.model.bare_alias_for_high))

    return {
      key: `${acc.platform}:${acc.baseName}`,
      baseName: acc.baseName,
      platform: acc.platform,
      isFolded: true,
      variants,
      defaultEffort: defaultVariant.effort,
      defaultModel: defaultVariant.model,
      models: variants.map((v) => v.model),
      bareAliasForHigh,
      bareAliasName: bareAliasForHigh ? acc.baseName : null
    }
  }

  for (const model of models) {
    if (deduplicatedBareModels.has(model)) {
      const bareKey = familyBucketKey(model.platform, model.name.trim())
      const acc = activeFoldedFamilies.get(bareKey)
      if (acc && !emittedFamilies.has(bareKey)) {
        emittedFamilies.add(bareKey)
        entries.push(buildFoldedEntry(acc))
      }
      continue
    }

    const parsed = parseEffortSuffix(model.name)
    if (parsed) {
      const key = familyBucketKey(model.platform, parsed.baseName)
      const acc = activeFoldedFamilies.get(key)
      if (acc) {
        if (!emittedFamilies.has(key)) {
          emittedFamilies.add(key)
          entries.push(buildFoldedEntry(acc))
        }
        continue
      }
    }

    const bareAliasForHigh = Boolean(model.bare_alias_for_high)
    entries.push({
      key: `${model.platform}:${model.name}`,
      baseName: model.name,
      platform: model.platform,
      isFolded: false,
      variants: [],
      defaultEffort: null,
      defaultModel: model,
      models: [model],
      bareAliasForHigh,
      bareAliasName: bareAliasForHigh ? (parsed?.baseName ?? model.name) : null
    })
  }

  return entries
}

/**
 * Resolves the active PlazaModel for a folded entry given an optional selected effort level.
 */
export function resolveVariantForEntry(
  entry: PlazaFoldedEntry,
  selectedEffort?: PlazaEffortLevel | null
): PlazaModel {
  if (!entry.isFolded || !selectedEffort) {
    return entry.defaultModel
  }
  const matched = entry.variants.find((v) => v.effort === selectedEffort)
  return matched ? matched.model : entry.defaultModel
}

/**
 * Finds the folded multi-effort family that contains `modelName` (or matches its bare alias).
 * Returns null if `modelName` is not part of a folded (>= 2 variants) effort family.
 */
export function findEffortGroupForModel(
  models: readonly PlazaModel[],
  modelName: string,
  platform?: string | null
): PlazaFoldedEntry | null {
  const target = (modelName || '').trim().toLowerCase()
  if (!target) return null
  const normPlatform = platform?.trim().toLowerCase() || ''

  const folded = foldPlazaModels(models)
  for (const entry of folded) {
    if (!entry.isFolded) continue
    if (normPlatform && entry.platform.trim().toLowerCase() !== normPlatform) continue

    if (entry.variants.some((v) => v.model.name.toLowerCase() === target)) {
      return entry
    }
    if (entry.bareAliasForHigh && entry.baseName.toLowerCase() === target) {
      return entry
    }
  }
  return null
}
