import { z } from 'zod'
import { apiEnvelopeSchema, apiErrorSchema } from './schemas'

const BASE_URL = import.meta.env.VITE_API_BASE_URL ?? ''

export class ApiRequestError extends Error {
  constructor(
    public readonly status: number,
    public readonly body: z.infer<typeof apiErrorSchema>,
  ) {
    super(body.error)
    this.name = 'ApiRequestError'
  }
}

export class ApiContractError extends Error {
  constructor(
    public readonly path: string,
    public readonly issues: z.core.$ZodIssue[],
  ) {
    super(`Unexpected ${path} response: ${issues.map((issue) => issue.message).join('; ')}`)
    this.name = 'ApiContractError'
  }
}

export async function request<T extends z.ZodType>(
  path: string,
  schema: T,
  params?: Record<string, string | string[] | number | undefined>,
): Promise<{ data: z.output<T>; nextToken?: string }> {
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
    const raw = await response.json().catch(() => ({
      error: response.statusText,
      code: response.status,
    }))
    const body = apiErrorSchema.catch({ error: response.statusText, code: response.status }).parse(raw)
    throw new ApiRequestError(response.status, body)
  }

  const parsed = apiEnvelopeSchema(schema).safeParse(await response.json())
  if (!parsed.success) throw new ApiContractError(path, parsed.error.issues)
  const envelope = parsed.data as { data: z.output<T>; next_token?: string }
  return { data: envelope.data, nextToken: envelope.next_token }
}
