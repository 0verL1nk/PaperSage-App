declare module './latex.mjs' {
  import { LRParser } from '@lezer/lr'
  export const parser: LRParser
}

declare module './latex.terms.mjs' {
  export const SectionCtrlSeq: number
  export const SubSectionCtrlSeq: number
  export const SubSubSectionCtrlSeq: number
  export const ChapterCtrlSeq: number
  export const PartCtrlSeq: number
  export const ParagraphCtrlSeq: number
  export const SubParagraphCtrlSeq: number
  export const BookCtrlSeq: number
  export const Begin: number
  export const End: number
  export const EnvName: number
  export const CtrlSeq: number
  export const DocumentEnvName: number
}