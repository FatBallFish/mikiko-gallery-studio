import { createRedeemCodeSubmitter, normalizeRedeemCode } from './redeemCodeState'

if (normalizeRedeemCode('  CODE-123  ') !== 'CODE-123' || normalizeRedeemCode('   ') !== '') {
  throw new Error('redeem code normalization should trim user input')
}

let redeemCalls = 0
let successCalls = 0
let releaseRedeem: (() => void) | undefined
const submit = createRedeemCodeSubmitter({
  redeem: async (code) => {
    if (code !== 'CODE-123') throw new Error(`unexpected normalized code ${code}`)
    redeemCalls += 1
    await new Promise<void>((resolve) => { releaseRedeem = resolve })
  },
  onSuccess: async () => { successCalls += 1 },
})

const invalid = await submit('   ')
if (invalid !== 'invalid' || redeemCalls !== 0) {
  throw new Error('blank redeem code should not call the API')
}

const first = submit('  CODE-123  ')
const duplicate = await submit('CODE-123')
if (duplicate !== 'busy' || Number(redeemCalls) !== 1) {
  throw new Error(`duplicate submission should share a busy guard, calls=${redeemCalls} result=${duplicate}`)
}
releaseRedeem?.()
if (await first !== 'success' || Number(successCalls) !== 1) {
  throw new Error(`successful redemption should invoke one success callback, successCalls=${successCalls}`)
}
