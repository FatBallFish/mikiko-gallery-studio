import { adminButton, adminSurface, adminTokens } from './classes'

if (adminTokens.radius.sm !== '8px' || adminTokens.radius.md !== '10px' || adminTokens.radius.lg !== '14px') {
  throw new Error(`admin radius tokens should use low-noise values, got ${JSON.stringify(adminTokens.radius)}`)
}

if (adminSurface.card.includes('rounded-3xl') || adminSurface.lane.includes('rounded-3xl')) {
  throw new Error('admin surfaces should not default to rounded-3xl')
}

if (adminButton.primary.includes('30px') || adminButton.primary.includes('shadow-[')) {
  throw new Error(`primary button should not use large glow shadow, got ${adminButton.primary}`)
}
