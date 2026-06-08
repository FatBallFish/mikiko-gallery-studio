export type UserPointAdjustmentInput = {
  changePoints: string
  reason: string
  idempotencyKey: string
}

export function canSaveUserPointAdjustment(input: UserPointAdjustmentInput): boolean {
  return Boolean(input.changePoints.trim() && input.reason.trim() && input.idempotencyKey.trim())
}

export function newUserPointAdjustmentKey(userID: string | number, randomID = defaultRandomID): string {
  return `admin-user-${userID}-points-${randomID()}`
}

function defaultRandomID(): string {
  return typeof crypto !== 'undefined' && 'randomUUID' in crypto ? crypto.randomUUID() : `${Date.now()}-${Math.random().toString(36).slice(2)}`
}
