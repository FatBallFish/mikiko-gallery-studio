import { DependencyList, useCallback, useEffect, useState } from 'react'
import { errorMessage } from '../../shared/http-client'

export type ResourceState<T> = {
  data: T | null
  loading: boolean
  error: string | null
  reload: () => Promise<void>
}

export function useApiResource<T>(loader: () => Promise<T>, deps: DependencyList = []): ResourceState<T> {
  const [data, setData] = useState<T | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const reload = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      setData(await loader())
    } catch (caught) {
      setError(errorMessage(caught))
    } finally {
      setLoading(false)
    }
  }, deps)

  useEffect(() => {
    void reload()
  }, [reload])

  return { data, loading, error, reload }
}

export { errorMessage }
