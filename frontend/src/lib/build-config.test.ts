import { describe, expect, it } from 'vitest'
import { resolveBuildVersion } from './build-version'

describe('frontend build version', () => {
  it('uses the exact HEAD SemVer tag over a conflicting injected version', () => {
    expect(resolveBuildVersion({ tags: 'other\nv1.2.3\n', commit: '1234567890abcdef', explicit: 'v9.9.9', gitAvailable: true })).toBe('v1.2.3')
  })

  it('does not claim a release for an untagged HEAD', () => {
    expect(resolveBuildVersion({ tags: 'not-a-version', commit: '1234567890abcdef', explicit: 'v9.9.9', gitAvailable: true })).toBe('dev+1234567890ab')
  })

  it('does not claim a release from environment values without Git tag evidence', () => {
    expect(resolveBuildVersion({ commit: '1234567890abcdef', explicit: 'v2.0.0', gitAvailable: false })).toBe('dev+1234567890ab')
  })

  it('uses dev when Git and commit metadata are unavailable', () => {
    expect(resolveBuildVersion({ gitAvailable: false })).toBe('dev')
  })
})
