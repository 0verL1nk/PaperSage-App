import { describe, expect, it } from 'vitest'

import { ACCEPTED_FILE_TYPES, ACCEPTED_FORMATS_LABEL, FILE_TYPE_LABELS, getShortType } from './constants'

describe('upload file formats', () => {
  it('accepts markdown files alongside PDF, DOCX and TXT', () => {
    expect(ACCEPTED_FILE_TYPES['text/markdown']).toEqual(['.md'])
    expect(ACCEPTED_FORMATS_LABEL).toBe('PDF、DOCX、TXT、MD')
    expect(FILE_TYPE_LABELS).toContainEqual({ value: 'text/markdown', label: 'MD' })
  })

  it('labels markdown documents as MD in the table', () => {
    expect(getShortType('text/markdown')).toBe('MD')
  })
})
