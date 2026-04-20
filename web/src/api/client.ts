import type { ApiError } from '@/types/api'

const BASE_URL = import.meta.env.VITE_API_BASE_URL ?? ''

export class ApiRequestError extends Error {
  constructor(
    public readonly status: number,
    public readonly body: ApiError,
  ) {
    super(body.error)
    this.name = 'ApiRequestError'
  }
}

export async function request<T>(
  path: string,
  params?: Record<string, string | string[] | number | undefined>,
): Promise<T> {
  const url = new URL(`${BASE_URL}${path}`, window.location.origin)

  if (params) {
    for (const [key, value] of Object.entries(params)) {
      if (value === undefined) continue
      if (Array.isArray(value)) {
        for (const v of value) {
          url.searchParams.append(key, v)
        }
      } else {
        url.searchParams.set(key, String(value))
      }
    }
  }

  const response = await fetch(url.toString())

  if (!response.ok) {
    const body = (await response.json().catch(() => ({
      error: response.statusText,
      code: response.status,
    }))) as ApiError
    throw new ApiRequestError(response.status, body)
  }

  const json = (await response.json()) as { data: T }
  return json.data
}
