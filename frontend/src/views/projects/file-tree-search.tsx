import { useState, useRef, useEffect } from 'react'
import { Search, X } from 'lucide-react'

interface FileTreeSearchProps {
  value: string
  onChange: (value: string) => void
}

export function FileTreeSearch({ value, onChange }: FileTreeSearchProps) {
  const inputRef = useRef<HTMLInputElement>(null)
  const [focused, setFocused] = useState(false)

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === 'f' && !e.shiftKey) {
        e.preventDefault()
        inputRef.current?.focus()
      }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [])

  return (
    <div className="relative px-2 py-1.5" data-testid="file-tree-search">
      <div className={`flex items-center gap-1.5 px-2 py-1 rounded-md border transition-colors ${focused ? 'border-primary bg-background' : 'border-transparent bg-muted/50'}`}>
        <Search size={12} className="shrink-0 text-muted-foreground" />
        <input
          ref={inputRef}
          type="text"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          onFocus={() => setFocused(true)}
          onBlur={() => setFocused(false)}
          placeholder="搜索文件..."
          data-testid="file-tree-search-input"
          className="w-full bg-transparent text-[11px] outline-none placeholder:text-muted-foreground/50"
        />
        {value && (
          <button
            onClick={() => onChange('')}
            data-testid="file-tree-search-clear"
            className="shrink-0 text-muted-foreground hover:text-foreground transition-colors"
          >
            <X size={12} />
          </button>
        )}
      </div>
    </div>
  )
}
