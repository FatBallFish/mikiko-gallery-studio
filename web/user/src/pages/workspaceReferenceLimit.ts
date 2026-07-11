export function remainingReferenceCapacity(maximum: number, currentCount: number) {
  return Math.max(0, Math.floor(maximum) - Math.max(0, Math.floor(currentCount)))
}

export function limitReferenceSelection<T>(items: T[], remaining: number) {
  const capacity = Math.max(0, Math.floor(remaining))
  const accepted = items.slice(0, capacity)
  return {
    accepted,
    rejectedCount: Math.max(0, items.length - accepted.length),
  }
}

export function singleReferenceAddition<T>(item: T, maximum: number, currentCount: number) {
  const remaining = remainingReferenceCapacity(maximum, currentCount)
  const limited = limitReferenceSelection([item], remaining)
  return {
    item: limited.accepted[0] ?? null,
    remaining,
    rejected: limited.rejectedCount > 0,
  }
}
