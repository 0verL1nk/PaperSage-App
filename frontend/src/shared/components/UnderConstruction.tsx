/**
 * UnderConstruction - placeholder for features not yet implemented
 */
import { Construction } from 'lucide-react'

interface UnderConstructionProps {
  title?: string
  description?: string
}

export function UnderConstruction({
  title = 'Under Construction',
  description = 'This feature is coming soon',
}: UnderConstructionProps) {
  return (
    <div className="flex h-full items-center justify-center">
      <div className="text-center">
        <div className="mb-4 flex justify-center">
          <div className="rounded-full bg-muted p-4">
            <Construction className="h-8 w-8 text-muted-foreground" />
          </div>
        </div>
        <h1 className="text-2xl font-bold">{title}</h1>
        <p className="mt-2 text-muted-foreground">{description}</p>
      </div>
    </div>
  )
}
