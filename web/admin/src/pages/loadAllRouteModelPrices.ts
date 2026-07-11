import type { RouteModelPrice } from '../../../shared/api-types'

type PricePageQuery = { page: number; page_size: number }

export async function loadAllRouteModelPrices(
  fetchPage: (query: PricePageQuery) => Promise<RouteModelPrice[]>,
  pageSize = 100,
) {
  const prices: RouteModelPrice[] = []

  for (let page = 1; ; page += 1) {
    const items = await fetchPage({ page, page_size: pageSize })
    prices.push(...items)
    if (items.length < pageSize) return prices
  }
}
