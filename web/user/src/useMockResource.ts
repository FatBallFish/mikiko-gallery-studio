import { useCallback, useEffect, useState } from 'react'
import type { DependencyList, Dispatch, SetStateAction } from 'react'

export type ResourceState<T> = {
  data: T | null
  loading: boolean
  error: string | null
  reload: () => Promise<void>
  setData: Dispatch<SetStateAction<T | null>>
}

function getMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error)
}

export function useMockResource<T>(loader: () => Promise<T>, deps: DependencyList = []): ResourceState<T> {
  const [data, setData] = useState<T | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const reload = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      setData(await loader())
    } catch (err) {
      setError(getMessage(err))
    } finally {
      setLoading(false)
    }
  }, deps)

  useEffect(() => {
    void reload()
  }, [reload])

  return { data, loading, error, reload, setData }
}

export function errorMessage(error: unknown) {
  return getMessage(error)
}
