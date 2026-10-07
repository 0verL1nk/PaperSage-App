import { expect, test } from '@playwright/test'

test('local guest workspace survives reload with writing state and file blobs', async ({ page }) => {
  const apiRequests: string[] = []
  await page.route('**/api/**', async (route) => {
    if (route.request().url().includes('/api/v1/auth/email-domains')) {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 200, data: { domains: [] } }) })
      return
    }
    apiRequests.push(route.request().url())
    await route.abort()
  })
  await page.goto('/auth')

  const created = await page.evaluate(async () => {
    const local = await import('/src/services/guest-local-workspace.ts')
    const project = await local.createGuestProject({ name: 'Local persistence check', scenario: 'undergraduate_thesis' })
    const state = await local.getGuestWritingState(project.id)
    state.brief.topic = '存储回读主题'
    await local.saveGuestWritingState(project.id, {
      brief: state.brief,
      requirements: state.requirements,
      phases: state.phases,
      current_phase: 'prepare',
      revision: state.revision,
    })
    const folder = await local.createGuestFolder({ projectId: project.id, name: 'source' })
    const doc = await local.saveGuestDocument(project.id, new File(['blob-data'], 'notes.txt', { type: 'text/plain' }), null, 'test')
    const latex = await local.createGuestLatexBlobFile(project.id, 'assets/pixel.pdf', new File([Uint8Array.from([0, 1, 255, 10])], 'pixel.pdf', { type: 'application/pdf' }))
    const accountId = '6fcf7435-4d69-4e7e-a148-edc00cb1d947'
    const initialBundle = await local.exportGuestWorkspace()
    await local.recordGuestProjectImport(initialBundle.bundleId, accountId, project.id, 'c3eaf25d-ad26-4654-bde7-f40cf640d60d')
    await local.recordGuestFolderImport(initialBundle.bundleId, accountId, project.id, folder.id, '271720fb-dae8-4e19-a13a-1a7260f6d72d')
    await local.recordGuestDocumentImport(initialBundle.bundleId, accountId, project.id, doc.id, '442ea227-071e-473f-b810-46010ccdeab1')
    await local.recordGuestLatexImport(initialBundle.bundleId, accountId, project.id, latex.file_path)
    const bundle = await local.exportGuestWorkspace()
    const headers = local.guestImportHeaders(bundle.bundleId, project.id)
    const { useSessionStore } = await import('/src/store/session-store.ts')
    useSessionStore.getState().activateLocalGuest()
    const agent = await import('/src/services/agent-api.ts')
    const latexApi = await import('/src/services/latex-api.ts')
    let aiBoundary = ''
    let compileBoundary = ''
    try { await agent.listConversations(project.id) } catch (error) { aiBoundary = error instanceof Error ? error.message : '' }
    try { await latexApi.compileLatex(project.id, [], 'main.tex', 'xelatex') } catch (error) { compileBoundary = error instanceof Error ? error.message : '' }
    return { projectId: project.id, documentId: doc.id, folderId: folder.id, bundleId: bundle.bundleId, aiBoundary, compileBoundary, headers }
  })

  await page.reload()
  const persisted = await page.evaluate(async ({ projectId, documentId, folderId, bundleId }) => {
    const local = await import('/src/services/guest-local-workspace.ts')
    const project = await local.getGuestProject(projectId)
    const writingState = await local.getGuestWritingState(projectId)
    const documentBlob = await local.getGuestDocumentBlob(documentId)
    const latexFile = await local.getGuestLatexFile(projectId, 'assets/pixel.pdf')
    const latexBlob = await local.getGuestLatexBlob(projectId, 'assets/pixel.pdf')
    const bundle = await local.exportGuestWorkspace()
    const mapping = await local.getGuestImportProgress(bundleId, '6fcf7435-4d69-4e7e-a148-edc00cb1d947')
    return {
      projectName: project.name,
      topic: writingState.brief.topic,
      documentText: await documentBlob.text(),
      latexContent: latexFile.content,
      latexBytes: Array.from(new Uint8Array(await latexBlob!.arrayBuffer())),
      sameBundleId: bundle.bundleId === bundleId,
      documentInBundle: bundle.projects[0]?.documents.some((item) => item.document.id === documentId),
      folderInBundle: bundle.projects[0]?.folders.some((item) => item.id === folderId),
      latexInBundle: bundle.projects[0]?.latexFiles.some((item) => item.file.file_path === 'assets/pixel.pdf' && item.blob?.size === 4),
      projectImportIdempotentMapping: mapping[0]?.cloudProjectId === 'c3eaf25d-ad26-4654-bde7-f40cf640d60d',
      folderImportIdempotentMapping: mapping[0]?.completedFolderIds[0]?.localId === folderId,
      documentImportIdempotentMapping: mapping[0]?.completedDocumentIds[0]?.localId === documentId,
      latexImportIdempotentMapping: mapping[0]?.completedLatexPaths.includes('assets/pixel.pdf'),
    }
  }, created)

  expect(persisted).toEqual({
    projectName: 'Local persistence check',
    topic: '存储回读主题',
    documentText: 'blob-data',
    latexContent: '',
    latexBytes: [0, 1, 255, 10],
    sameBundleId: true,
    documentInBundle: true,
    folderInBundle: true,
    latexInBundle: true,
    projectImportIdempotentMapping: true,
    folderImportIdempotentMapping: true,
    documentImportIdempotentMapping: true,
    latexImportIdempotentMapping: true,
  })
  expect(created.headers).toEqual({
    'X-PaperSage-Import-Bundle-Id': created.bundleId,
    'X-PaperSage-Import-Local-Id': created.projectId,
  })
  expect(created.aiBoundary).toContain('AI 助手需要登录')
  expect(created.compileBoundary).toContain('LaTeX 在线编译需要登录')
  expect(apiRequests).toEqual([])
})
