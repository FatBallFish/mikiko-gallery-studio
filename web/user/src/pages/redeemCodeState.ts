export type RedeemCodeSubmitResult = 'success' | 'invalid' | 'busy'

export function normalizeRedeemCode(code: string) {
  return code.trim()
}

export function createRedeemCodeSubmitter({
  redeem,
  onSuccess,
}: {
  redeem: (code: string) => Promise<unknown>
  onSuccess: () => void | Promise<void>
}) {
  let busy = false
  return async (input: string): Promise<RedeemCodeSubmitResult> => {
    const code = normalizeRedeemCode(input)
    if (!code) return 'invalid'
    if (busy) return 'busy'
    busy = true
    try {
      await redeem(code)
      await onSuccess()
      return 'success'
    } finally {
      busy = false
    }
  }
}
