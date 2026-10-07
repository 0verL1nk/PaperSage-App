import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/services', () => ({
  get: vi.fn(),
  put: vi.fn(),
}))

import { get, put } from '@/services'
import { WRITING_STATE } from './api-routes'
import { getConflictState, useSaveWritingState, useWritingState } from './writing-state-api'

describe('writing state API', () => {
  beforeEach(() => {
    vi.mocked(get).mockReset()
    vi.mocked(put).mockReset()
  })

  it('builds the project-scoped endpoint', () => {
    expect(WRITING_STATE.project('project-1')).toBe('/api/v1/projects/project-1/writing-state')
  })

  it('exports query and save hooks', () => {
    expect(useWritingState).toBeTypeOf('function')
    expect(useSaveWritingState).toBeTypeOf('function')
  })

  it('reads the latest state from a 409 envelope', () => {
    const latest = { revision: 3, current_phase: 'write' }
    const error = Object.assign(new Error('conflict'), {
      details: { code: 409, data: latest, info: 'conflict' },
    })
    expect(getConflictState(error)).toBe(latest)
    expect(getConflictState(new Error('network'))).toBeNull()
  })

  it('does not treat a 404 payload as a conflict state', () => {
    const error = Object.assign(new Error('missing'), { details: { code: 404, data: null } })
    expect(getConflictState(error)).toBeNull()
  })
})
