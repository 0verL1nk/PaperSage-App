/**
 * CreateProjectDialog - dialog for creating a new project
 */
import { useState } from 'react'
import { useCreateProject } from '../api'
import type { ProjectGroup } from '../types'

interface CreateProjectDialogProps {
  /** Whether the dialog is open */
  open: boolean
  /** Callback when open state changes */
  onOpenChange: (open: boolean) => void
  /** Available groups */
  groups: ProjectGroup[]
}

export function CreateProjectDialog({ open, onOpenChange, groups }: CreateProjectDialogProps) {
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [groupId, setGroupId] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  const createProject = useCreateProject()
  const isPending = createProject.isPending

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const handleSubmit = async (e: any) => {
    e.preventDefault()
    e.preventDefault()
    if (!name.trim()) return

    setError(null)

    try {
      await createProject.mutateAsync({
        name: name.trim(),
        description: description.trim() || undefined,
        group_id: groupId ?? undefined,
      })
      // Reset form
      setName('')
      setDescription('')
      setGroupId(null)
      onOpenChange(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create project')
    }
  }

  const handleOpenChange = (isOpen: boolean) => {
    if (!isOpen) {
      // Reset form when closing
      setName('')
      setDescription('')
      setGroupId(null)
      setError(null)
    }
    onOpenChange(isOpen)
  }

  if (!open) return null

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-background/80 animate-dialog-overlay"
      onClick={() => handleOpenChange(false)}
      role="dialog"
      aria-modal="true"
    >
      <div
        className="w-full max-w-md p-6 rounded-lg border bg-card animate-dialog-content shadow-lg"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="text-lg font-semibold mb-4">Create New Project</h2>

        <form onSubmit={handleSubmit} className="space-y-4">
          {error && (
            <div className="p-3 rounded-md bg-destructive/10 text-destructive text-sm">
              {error}
            </div>
          )}
          <div>
            <label htmlFor="name" className="block text-sm font-medium mb-1">
              Name <span className="text-destructive">*</span>
            </label>
            <input
              id="name"
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="My Project"
              className="w-full px-3 py-2 rounded-md border bg-background"
              required
              autoFocus
            />
          </div>

          <div>
            <label htmlFor="description" className="block text-sm font-medium mb-1">
              Description
            </label>
            <textarea
              id="description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Project description (optional)"
              className="w-full px-3 py-2 rounded-md border bg-background resize-none"
              rows={3}
            />
          </div>

          {groups.length > 0 && (
            <div>
              <label htmlFor="group" className="block text-sm font-medium mb-1">
                Group
              </label>
              <select
                id="group"
                value={groupId ?? ''}
                onChange={(e) => setGroupId(e.target.value || null)}
                className="w-full px-3 py-2 rounded-md border bg-background"
              >
                <option value="">No group</option>
                {groups.map((group) => (
                  <option key={group.id} value={group.id}>
                    {group.name}
                  </option>
                ))}
              </select>
            </div>
          )}

          <div className="flex justify-end gap-2 pt-2">
            <button
              type="button"
              onClick={() => handleOpenChange(false)}
              className="px-4 py-2 rounded-md border hover:bg-accent"
              disabled={isPending}
            >
              Cancel
            </button>
            <button
              type="submit"
              className="px-4 py-2 rounded-md bg-primary text-primary-foreground hover:bg-primary/90"
              disabled={isPending || !name.trim()}
            >
              {isPending ? 'Creating...' : 'Create'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}