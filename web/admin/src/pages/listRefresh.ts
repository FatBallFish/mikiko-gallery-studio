export function createLatestListRequestGuard() {
  let generation = 0
  return {
    begin: () => ++generation,
    isCurrent: (request: number) => request === generation,
    invalidate: () => { generation += 1 },
  }
}
