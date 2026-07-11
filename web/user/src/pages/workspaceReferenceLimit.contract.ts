import { limitReferenceSelection, remainingReferenceCapacity, singleReferenceAddition } from './workspaceReferenceLimit'

const capacityCases = [
  { max: 5, current: 2, expected: 3 },
  { max: 5, current: 5, expected: 0 },
  { max: 3, current: 7, expected: 0 },
  { max: 0, current: 0, expected: 0 },
]

for (const testCase of capacityCases) {
  const actual = remainingReferenceCapacity(testCase.max, testCase.current)
  if (actual !== testCase.expected) {
    throw new Error(`remaining reference capacity mismatch for ${JSON.stringify(testCase)}, got ${actual}`)
  }
}

const partial = limitReferenceSelection(['a', 'b', 'c', 'd'], 2)
if (JSON.stringify(partial.accepted) !== JSON.stringify(['a', 'b']) || partial.rejectedCount !== 2) {
  throw new Error(`reference selection must preserve order and slice to remaining capacity, got ${JSON.stringify(partial)}`)
}

const full = limitReferenceSelection(['a', 'b'], 3)
if (full.accepted.length !== 2 || full.rejectedCount !== 0) {
  throw new Error(`selection within capacity must remain intact, got ${JSON.stringify(full)}`)
}

const zero = limitReferenceSelection(['a', 'b'], 0)
if (zero.accepted.length !== 0 || zero.rejectedCount !== 2) {
  throw new Error(`zero capacity must reject every incoming reference, got ${JSON.stringify(zero)}`)
}

const historyAtLimit = singleReferenceAddition('generated-image', 2, 2)
if (historyAtLimit.item !== null || historyAtLimit.remaining !== 0 || !historyAtLimit.rejected) {
  throw new Error(`history edit source must be rejected before upload at the live limit, got ${JSON.stringify(historyAtLimit)}`)
}

const historyWithCapacity = singleReferenceAddition('generated-image', 3, 2)
if (historyWithCapacity.item !== 'generated-image' || historyWithCapacity.remaining !== 1 || historyWithCapacity.rejected) {
  throw new Error(`history edit source must be accepted within live capacity, got ${JSON.stringify(historyWithCapacity)}`)
}
