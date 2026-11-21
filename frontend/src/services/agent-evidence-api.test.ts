import { beforeEach, describe, expect, it, vi } from 'vitest'

const client = vi.hoisted(() => ({ getJson: vi.fn() }))
vi.mock('@/services/client', () => ({
  getJson: client.getJson,
  postJson: vi.fn(),
  deleteFetch: vi.fn(),
}))

import { getActiveRun, listRunEvidence } from '@/services/agent-api'
import { AGENT } from '@/services/api-routes'

describe('listRunEvidence', () => {
  beforeEach(() => client.getJson.mockReset())

  it('reads the persisted evidence DTO from the authenticated Run endpoint', async () => {
    const rows = [{ id: 'e1', sourceName: 'research-plan.pdf', quote: 'exact excerpt' }]
    client.getJson.mockResolvedValue(rows)

    await expect(listRunEvidence('run-1')).resolves.toEqual(rows)
    expect(client.getJson).toHaveBeenCalledWith(AGENT.runEvidence('run-1'), undefined)
  })

  it('allows the active-run lookup to refresh an expired session', async () => {
    client.getJson.mockResolvedValue({ runId: 'run-1' })

    await expect(getActiveRun('conversation-1')).resolves.toEqual({ runId: 'run-1' })
    expect(client.getJson).toHaveBeenCalledWith(AGENT.activeRun('conversation-1'), undefined)
  })
})
