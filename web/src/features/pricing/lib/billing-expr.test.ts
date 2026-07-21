/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  BILLING_PRICING_VARS,
  normalizeTierLabel,
  parseTiersFromExpr,
  type ParsedTier,
} from './billing-expr.ts'

describe('parseTiersFromExpr', () => {
  describe('channel() based expressions', () => {
    const channelExpr =
      'channel("name") == "GPTProto" ? tier("per-call", 680000) : ' +
      'channel("name") == "万界方舟" ? tier("token", p * 14.55 + c * 872.71) : ' +
      'channel("name") == "UCloud" ? tier("combo", p * 17.04 + c * 102.53 + ao * 102.53 + img_o * 1023.6) :' +
      'tier("per-call", 680000)'

    test('parses all tiers from channel-conditional expression', () => {
      const tiers = parseTiersFromExpr(channelExpr)
      assert.equal(tiers.length, 3)
      assert.equal(tiers[0].label, 'per-call')
      assert.equal(tiers[1].label, 'token')
      assert.equal(tiers[2].label, 'combo')
    })

    test('per-call tier has fixedCost and no variable coefficients', () => {
      const tiers = parseTiersFromExpr(channelExpr)
      const perCall = tiers[0]
      assert.equal(perCall['fixedCost'], 680000)
      assert.equal(perCall['inputPrice'], 0)
      assert.equal(perCall['outputPrice'], 0)
    })

    test('token tier has correct input/output coefficients', () => {
      const tiers = parseTiersFromExpr(channelExpr)
      const token = tiers[1]
      assert.equal(token['inputPrice'], 14.55)
      assert.equal(token['outputPrice'], 872.71)
      assert.equal(token['fixedCost'], undefined)
    })

    test('combo tier has correct multi-variable coefficients', () => {
      const tiers = parseTiersFromExpr(channelExpr)
      const combo = tiers[2]
      assert.equal(combo['inputPrice'], 17.04)
      assert.equal(combo['outputPrice'], 102.53)
      assert.equal(combo['audioOutputPrice'], 102.53)
      assert.equal(combo['imageOutputPrice'], 1023.6)
      assert.equal(combo['fixedCost'], undefined)
    })

    test('deduplicates tiers with same label', () => {
      const tiers = parseTiersFromExpr(channelExpr)
      const perCallTiers = tiers.filter((t) => t.label === 'per-call')
      assert.equal(perCallTiers.length, 1)
    })
  })

  describe('traditional p/c/len condition expressions', () => {
    const traditionalExpr =
      'p <= 200000 ? tier("standard", p * 1.5 + c * 7.5) : tier("long_context", p * 3 + c * 11.25)'

    test('parses traditional tier conditions', () => {
      const tiers = parseTiersFromExpr(traditionalExpr)
      assert.equal(tiers.length, 2)
      assert.equal(tiers[0].label, 'standard')
      assert.equal(tiers[1].label, 'long_context')
    })

    test('standard tier has correct coefficients', () => {
      const tiers = parseTiersFromExpr(traditionalExpr)
      assert.equal(tiers[0]['inputPrice'], 1.5)
      assert.equal(tiers[0]['outputPrice'], 7.5)
    })

    test('long_context tier has correct coefficients', () => {
      const tiers = parseTiersFromExpr(traditionalExpr)
      assert.equal(tiers[1]['inputPrice'], 3)
      assert.equal(tiers[1]['outputPrice'], 11.25)
    })
  })

  describe('simple flat expression', () => {
    test('parses single tier', () => {
      const tiers = parseTiersFromExpr('tier("base", p * 2.5 + c * 15)')
      assert.equal(tiers.length, 1)
      assert.equal(tiers[0].label, 'base')
      assert.equal(tiers[0]['inputPrice'], 2.5)
      assert.equal(tiers[0]['outputPrice'], 15)
    })

    test('parses per-call only expression', () => {
      const tiers = parseTiersFromExpr('tier("fixed", 500000)')
      assert.equal(tiers.length, 1)
      assert.equal(tiers[0].label, 'fixed')
      assert.equal(tiers[0]['fixedCost'], 500000)
    })
  })

  describe('edge cases', () => {
    test('returns empty array for empty string', () => {
      assert.deepEqual(parseTiersFromExpr(''), [])
    })

    test('returns empty array for null-ish input', () => {
      assert.deepEqual(parseTiersFromExpr(null as unknown as string), [])
      assert.deepEqual(parseTiersFromExpr(undefined as unknown as string), [])
    })

    test('handles expression with cache variables', () => {
      const tiers = parseTiersFromExpr(
        'tier("base", p * 3 + c * 15 + cr * 0.3 + cc * 3.75)'
      )
      assert.equal(tiers.length, 1)
      assert.equal(tiers[0]['inputPrice'], 3)
      assert.equal(tiers[0]['outputPrice'], 15)
      assert.equal(tiers[0]['cacheReadPrice'], 0.3)
      assert.equal(tiers[0]['cacheCreatePrice'], 3.75)
    })

    test('handles expression with image and audio variables', () => {
      const tiers = parseTiersFromExpr(
        'tier("multi", p * 2 + c * 10 + img * 5 + img_o * 100 + ai * 20 + ao * 40)'
      )
      assert.equal(tiers.length, 1)
      assert.equal(tiers[0]['imagePrice'], 5)
      assert.equal(tiers[0]['imageOutputPrice'], 100)
      assert.equal(tiers[0]['audioInputPrice'], 20)
      assert.equal(tiers[0]['audioOutputPrice'], 40)
    })

    test('handles versioned expression prefix', () => {
      const tiers = parseTiersFromExpr('v1:tier("base", p * 5 + c * 25)')
      assert.equal(tiers.length, 1)
      assert.equal(tiers[0].label, 'base')
      assert.equal(tiers[0]['inputPrice'], 5)
      assert.equal(tiers[0]['outputPrice'], 25)
    })

    test('handles nested parentheses in tier body', () => {
      const tiers = parseTiersFromExpr(
        'tier("complex", max(p * 2, 100) + c * 10)'
      )
      assert.equal(tiers.length, 1)
      assert.equal(tiers[0].label, 'complex')
      assert.equal(tiers[0]['outputPrice'], 10)
    })
  })
})

describe('normalizeTierLabel', () => {
  test('returns empty string for undefined/empty', () => {
    assert.equal(normalizeTierLabel(undefined), '')
    assert.equal(normalizeTierLabel(''), '')
  })

  test('normalizes unicode comparison operators', () => {
    assert.equal(normalizeTierLabel('p≤200K'), 'p<200k')
    assert.equal(normalizeTierLabel('p≥100K'), 'p>100k')
  })

  test('normalizes fullwidth operators', () => {
    assert.equal(normalizeTierLabel('p＜200K'), 'p<200k')
    assert.equal(normalizeTierLabel('p＞100K'), 'p>100k')
  })

  test('removes whitespace and lowercases', () => {
    assert.equal(normalizeTierLabel('Long Context'), 'longcontext')
    assert.equal(normalizeTierLabel('PER-CALL'), 'per-call')
  })
})

describe('evalExprLocally environment (inline simulation)', () => {
  function evalExpr(exprStr: string, p = 1000, c = 500) {
    let matchedTier = ''
    const env: Record<string, unknown> = {
      p,
      c,
      len: p,
      tier: (name: string, value: number) => {
        matchedTier = name
        return value
      },
      channel: () => '',
      header: () => '',
      param: () => null,
      has: (src: unknown, substr: string) =>
        typeof src === 'string' ? src.includes(substr) : false,
      hour: () => 0,
      minute: () => 0,
      weekday: () => 0,
      month: () => 0,
      day: () => 0,
      max: Math.max,
      min: Math.min,
      abs: Math.abs,
      ceil: Math.ceil,
      floor: Math.floor,
      cr: 0,
      cc: 0,
      cc1h: 0,
      img: 0,
      img_o: 0,
      ai: 0,
      ao: 0,
    }
    const fn = new Function(...Object.keys(env), `"use strict"; return (${exprStr});`)
    const cost = Number(fn(...Object.values(env))) || 0
    return { cost, matchedTier, error: null }
  }

  test('channel() expression falls back to default tier', () => {
    const expr =
      'channel("name") == "GPTProto" ? tier("per-call", 680000) : ' +
      '(channel("name") == "万界方舟" ? tier("token", p * 14.55 + c * 872.71) : ' +
      'tier("per-call", 680000))'
    const result = evalExpr(expr)
    assert.equal(result.matchedTier, 'per-call')
    assert.equal(result.cost, 680000)
  })

  test('header() returns empty string without error', () => {
    const expr = 'header("x-custom") == "fast" ? tier("fast", p * 4) : tier("normal", p * 2)'
    const result = evalExpr(expr)
    assert.equal(result.matchedTier, 'normal')
    assert.equal(result.cost, 2000)
  })

  test('time functions return 0 without error', () => {
    const expr = 'hour("UTC") >= 9 && hour("UTC") < 18 ? tier("peak", p * 3) : tier("off-peak", p * 1.5)'
    const result = evalExpr(expr)
    assert.equal(result.matchedTier, 'off-peak')
    assert.equal(result.cost, 1500)
  })

  test('has() function works for string matching', () => {
    const expr = 'has("hello world", "world") ? tier("match", 100) : tier("no-match", 0)'
    const result = evalExpr(expr)
    assert.equal(result.matchedTier, 'match')
    assert.equal(result.cost, 100)
  })

  test('simple token expression works', () => {
    const result = evalExpr('tier("base", p * 2.5 + c * 15)', 1000, 500)
    assert.equal(result.matchedTier, 'base')
    assert.equal(result.cost, 1000 * 2.5 + 500 * 15)
  })
})

describe('fixedCost integration (dynamic-price / format logic)', () => {
  const PRIMARY_DYNAMIC_FIELDS = new Set(['inputPrice', 'outputPrice', 'fixedCost'])

  function resolveMatchedTier(
    tiers: ParsedTier[],
    matchedLabel: string | undefined
  ): ParsedTier | null {
    if (tiers.length === 0) return null
    if (!matchedLabel) return null
    return (
      tiers.find((t) => {
        const l1 = normalizeTierLabel(t.label)
        const l2 = normalizeTierLabel(matchedLabel)
        return l1 === l2 && l1 !== ''
      }) || null
    )
  }

  function getTieredBillingSummaryEntries(
    tiers: ParsedTier[],
    tier: ParsedTier,
    hasCacheTokens: boolean
  ): Array<{ field: string; shortLabel: string; price: number }> {
    const entries: Array<{ field: string; shortLabel: string; price: number }> = []
    const fixedCost = Number(tier['fixedCost' as keyof ParsedTier] || 0)
    if (fixedCost > 0) {
      entries.push({ field: 'fixedCost', shortLabel: 'Per-call', price: fixedCost / 1_000_000 })
    } else {
      for (const v of BILLING_PRICING_VARS) {
        if (!v.field) continue
        if (v.group === 'cache' && !hasCacheTokens) continue
        const raw = tier[v.field as keyof ParsedTier]
        const price = Number(raw)
        if (Number.isFinite(price) && price > 0) {
          entries.push({ field: v.field, shortLabel: v.shortLabel, price })
        }
      }
    }
    return entries
  }

  test('per-call tier produces fixedCost entry with correct price', () => {
    const tiers = parseTiersFromExpr('tier("per-call", 680000)')
    const tier = resolveMatchedTier(tiers, 'per-call')!
    const entries = getTieredBillingSummaryEntries(tiers, tier, false)
    assert.equal(entries.length, 1)
    assert.equal(entries[0].field, 'fixedCost')
    assert.equal(entries[0].price, 0.68)
  })

  test('fixedCost is classified as primary field', () => {
    const tiers = parseTiersFromExpr('tier("per-call", 500000)')
    const tier = resolveMatchedTier(tiers, 'per-call')!
    const entries = getTieredBillingSummaryEntries(tiers, tier, false)
    const primaryEntries = entries.filter((e) => PRIMARY_DYNAMIC_FIELDS.has(e.field))
    assert.equal(primaryEntries.length, 1)
    assert.equal(primaryEntries[0].field, 'fixedCost')
  })

  test('token tier does NOT produce fixedCost entry', () => {
    const tiers = parseTiersFromExpr('tier("token", p * 2.5 + c * 15)')
    const tier = resolveMatchedTier(tiers, 'token')!
    const entries = getTieredBillingSummaryEntries(tiers, tier, false)
    const fields = entries.map((e) => e.field)
    assert.ok(fields.includes('inputPrice'))
    assert.ok(fields.includes('outputPrice'))
    assert.ok(!fields.includes('fixedCost'))
  })

  test('channel-based per-call tier resolves and produces fixedCost', () => {
    const expr =
      'channel("name") == "GPTProto" ? tier("per-call", 680000) : tier("token", p * 2.5 + c * 15)'
    const tiers = parseTiersFromExpr(expr)
    const tier = resolveMatchedTier(tiers, 'per-call')!
    const entries = getTieredBillingSummaryEntries(tiers, tier, false)
    assert.equal(entries.length, 1)
    assert.equal(entries[0].field, 'fixedCost')
    assert.equal(entries[0].price, 0.68)
  })

  test('channel-based token tier resolves and produces normal entries', () => {
    const expr =
      'channel("name") == "GPTProto" ? tier("per-call", 680000) : tier("token", p * 2.5 + c * 15)'
    const tiers = parseTiersFromExpr(expr)
    const tier = resolveMatchedTier(tiers, 'token')!
    const entries = getTieredBillingSummaryEntries(tiers, tier, false)
    const fields = entries.map((e) => e.field)
    assert.ok(fields.includes('inputPrice'))
    assert.ok(fields.includes('outputPrice'))
    assert.ok(!fields.includes('fixedCost'))
  })

  test('cache entries suppressed when no cache tokens', () => {
    const tiers = parseTiersFromExpr('tier("base", p * 2 + c * 10 + cr * 0.5 + cc * 2)')
    const tier = resolveMatchedTier(tiers, 'base')!
    const entries = getTieredBillingSummaryEntries(tiers, tier, false)
    const fields = entries.map((e) => e.field)
    assert.ok(!fields.includes('cacheReadPrice'))
    assert.ok(!fields.includes('cacheCreatePrice'))
  })

  test('cache entries included when cache tokens present', () => {
    const tiers = parseTiersFromExpr('tier("base", p * 2 + c * 10 + cr * 0.5 + cc * 2)')
    const tier = resolveMatchedTier(tiers, 'base')!
    const entries = getTieredBillingSummaryEntries(tiers, tier, true)
    const fields = entries.map((e) => e.field)
    assert.ok(fields.includes('cacheReadPrice'))
    assert.ok(fields.includes('cacheCreatePrice'))
  })

  test('unmatched tier label returns null', () => {
    const tiers = parseTiersFromExpr('tier("base", p * 2 + c * 10)')
    assert.equal(resolveMatchedTier(tiers, 'nonexistent'), null)
  })

  test('multi-channel expression with 3 tiers all resolvable', () => {
    const expr =
      'channel("name") == "A" ? tier("per-call", 680000) : ' +
      'channel("name") == "B" ? tier("token", p * 14.55 + c * 872.71) : ' +
      'tier("combo", p * 17.04 + c * 102.53 + ao * 102.53 + img_o * 1023.6)'
    const tiers = parseTiersFromExpr(expr)
    assert.equal(tiers.length, 3)

    const perCall = resolveMatchedTier(tiers, 'per-call')!
    assert.equal(
      getTieredBillingSummaryEntries(tiers, perCall, false)[0].field,
      'fixedCost'
    )

    const token = resolveMatchedTier(tiers, 'token')!
    const tokenFields = getTieredBillingSummaryEntries(tiers, token, false).map((e) => e.field)
    assert.ok(tokenFields.includes('inputPrice'))
    assert.ok(tokenFields.includes('outputPrice'))

    const combo = resolveMatchedTier(tiers, 'combo')!
    const comboEntries = getTieredBillingSummaryEntries(tiers, combo, false)
    const comboFields = comboEntries.map((e) => e.field)
    assert.ok(comboFields.includes('inputPrice'))
    assert.ok(comboFields.includes('outputPrice'))
    assert.ok(comboFields.includes('audioOutputPrice'))
    assert.ok(comboFields.includes('imageOutputPrice'))
  })
})
