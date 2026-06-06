import { canSaveUserPointAdjustment, newUserPointAdjustmentKey } from './userPointAdjustment'

const valid = { changePoints: '10.00000', reason: '补偿测试用户', idempotencyKey: 'admin-user-42-points-fixed' }

if (!canSaveUserPointAdjustment(valid)) {
  throw new Error('user point adjustment should be saveable when points, reason and idempotency key are all present')
}

for (const input of [
  { ...valid, changePoints: '   ' },
  { ...valid, reason: '' },
  { ...valid, idempotencyKey: '' },
]) {
  if (canSaveUserPointAdjustment(input)) {
    throw new Error(`user point adjustment should require points, reason and idempotency key, got ${JSON.stringify(input)}`)
  }
}

const generated = newUserPointAdjustmentKey(42, () => 'uuid-123')
if (generated !== 'admin-user-42-points-uuid-123') {
  throw new Error(`user point adjustment idempotency key should use stable admin-user prefix, got ${generated}`)
}
