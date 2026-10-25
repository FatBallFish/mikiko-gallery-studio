export type ReferenceAdmissionPolicy<K extends string> = {
  limits: Partial<Record<K, number | null>>
  reason?: string
}
export type ReferenceAdmissionResult<T> = { ok: boolean; reason: string; added: T[]; merged: T[] }

/** Atomic admission. Unknown limits reject additions; existing references are never removed. */
export function referenceAdmission<T, K extends string>({ policy, current, incoming, reserved = {}, kind, key }: {
  policy: ReferenceAdmissionPolicy<K>; current: readonly T[]; incoming: readonly T[]
  reserved?: Partial<Record<K, number>>; kind: (item: T) => K; key?: (item: T) => string
}): ReferenceAdmissionResult<T> {
  const seen = new Set(key ? current.map(key) : [])
  const added = incoming.filter((item) => {
    if (!key) return true
    const id = key(item)
    if (seen.has(id)) return false
    seen.add(id); return true
  })
  const reject = (reason: string): ReferenceAdmissionResult<T> => ({ ok: false, reason, added: [], merged: [...current] })
  if (policy.reason) return reject(policy.reason)
  const counts = { ...reserved } as Partial<Record<K, number>>
  for (const item of [...current, ...added]) counts[kind(item)] = (counts[kind(item)] ?? 0) + 1
  for (const type of Object.keys(counts) as K[]) {
    const max = policy.limits[type]
    if (max == null || !Number.isFinite(max)) return reject(`当前模型的 ${type} 引用能力不可用，请手动选择支持的型号。`)
    if (max <= 0) return reject(`当前模型不支持 ${type} 引用。`)
    if ((counts[type] ?? 0) > max) return reject(`当前模型最多支持 ${max} 个 ${type} 引用（含正在添加的引用），本批未添加。`)
  }
  return { ok: true, reason: '', added, merged: [...current, ...added] }
}

/** Reserve synchronously before an async upload/import; revalidate completion excluding its own token. */
export function createReferenceReservations<K extends string>() {
  let sequence = 0
  const entries = new Map<number, Partial<Record<K, number>>>()
  function pending(excludeToken?: number) {
    const result: Partial<Record<K, number>> = {}
    for (const [token, counts] of entries) if (token !== excludeToken) {
      for (const type of Object.keys(counts) as K[]) result[type] = (result[type] ?? 0) + (counts[type] ?? 0)
    }
    return result
  }
  return {
    pending,
    release(token: number) { entries.delete(token) },
    reserve<T>(policy: ReferenceAdmissionPolicy<K>, current: readonly T[], incoming: readonly T[], kind: (item: T) => K, key?: (item: T) => string) {
      const result = referenceAdmission({ policy, current, incoming, kind, key, reserved: pending() })
      if (!result.ok) return { ...result, token: undefined }
      const counts: Partial<Record<K, number>> = {}
      for (const item of result.added) counts[kind(item)] = (counts[kind(item)] ?? 0) + 1
      const token = ++sequence
      entries.set(token, counts)
      return { ...result, token }
    },
  }
}
