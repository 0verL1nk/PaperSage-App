const semverTag = /^v?(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*))*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/

export function resolveBuildVersion({ tags, commit, gitAvailable }: {
  tags?: string
  commit?: string
  gitAvailable: boolean
  explicit?: string
}) {
  // An environment-provided version cannot prove that its tag points at HEAD.
  const tag = gitAvailable ? tags?.split(/\r?\n/).find((value) => semverTag.test(value)) : undefined
  if (tag) return tag
  return commit && /^[a-f\d]{7,64}$/i.test(commit) ? `dev+${commit.slice(0, 12)}` : 'dev'
}
