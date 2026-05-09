export type Stats = {
  total_quota: number
  total_used: number
  total_remaining: number
  key_count: number
  active_key_count: number
  today_requests: number
}

export type PoolStats = {
  total_quota: number
  total_used: number
  total_remaining: number
  active_key_count: number
}

export type TimeSeries = {
  granularity: string
  labels: string[]
  series: { name: string; data: number[] }[]
}

export type KeyItem = {
  id: number
  key: string
  alias: string
  total_quota: number
  used_quota: number
  is_active: boolean
  is_invalid: boolean
  is_donated: boolean
  last_used_at?: string | null
  created_at?: string
}

export type LogItem = {
  id: number
  request_id: string
  key_used: number
  key_alias: string
  endpoint: string
  status_code: number
  latency: number
  request_body?: string | null
  request_truncated?: boolean
  response_body?: string | null
  response_truncated?: boolean
  cache_hit?: boolean
  client_ip: string
  access_key_id: number
  access_key_alias: string
  created_at: string
}

export type AccessKeyItem = {
  id: number
  key: string
  alias: string
  is_active: boolean
  last_used_at?: string | null
  created_at?: string
}

export type LogStatusCount = {
  status_code: number
  count: number
}
