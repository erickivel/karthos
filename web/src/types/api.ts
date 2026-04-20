export interface ApiResponse<T> {
  data: T
}

export interface PaginatedData<T> {
  items: T[]
  total: number
}

export interface PaginatedResponse<T> {
  data: PaginatedData<T>
  next_token?: string
}

export interface ApiError {
  error: string
  code: number
}

export interface PaginationParams {
  page_size?: number
  next_token?: string
}
