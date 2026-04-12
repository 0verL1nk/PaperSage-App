/**
 * Project types - mirrors backend schemas
 */

export interface ProjectGroup {
  id: string
  name: string
  owner_id: string
  sort_order: number
  created_at: string
}

export interface Project {
  id: string
  name: string
  description: string | null
  owner_id: string
  group_id: string | null
  status: string
  is_deleted: boolean
  deleted_at: string | null
  created_at: string
  updated_at: string
  group?: ProjectGroup | null
}