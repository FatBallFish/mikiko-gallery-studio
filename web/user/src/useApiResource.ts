import { DependencyList, useCallback, useEffect, useState } from 'react'
import { errorMessage } from '../../shared/http-client'
import { useApp } from './components'

export type ResourceState<T> = {
  data: T | null
  loading: boolean
  error: string | null
  reload: () => Promise<void>
}

export function useApiResource<T>(loader: () => Promise<T>, deps: DependencyList = []): ResourceState<T> {
  const app = useApp()
  const [data, setData] = useState<T | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const reload = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      setData(await loader())
    } catch (caught) {
      const message = errorMessage(caught)
      setError(message)
      app.notify('error', message)
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
