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
