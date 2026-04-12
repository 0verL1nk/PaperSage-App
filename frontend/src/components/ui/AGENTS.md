# frontend/src/components/ui

shadcn/ui component library with Tailwind CSS 4, 19 ready-to-use components.

## WHERE TO LOOK

| Component | File | Notes |
|----------|------|-------|
| Button variants | `button.tsx` | class-variance-authority, 6 variants (default, outline, secondary, ghost, destructive, link) |
| Input styling | `input.tsx` | Uses border-input, ring-ring for focus states |
| Card subcomponents | `card.tsx` | Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter, CardAction |
| Dialog | `dialog.tsx` | Radix UI Dialog primitive |
| Sheet (slide-out) | `sheet.tsx` | Radix UI Sheet primitive |
| Dropdown | `dropdown-menu.tsx` | Radix UI DropdownMenu primitive |
| Form | `form.tsx` | React Hook Form + Zod integration |
| Table | `table.tsx` | Table, TableHeader, TableRow, TableCell, TableBody |
| Tabs | `tabs.tsx` | Radix UI Tabs primitive |
| Alert | `alert.tsx` | Alert, AlertTitle, AlertDescription |
| Badge | `badge.tsx` | Variants: default, secondary, outline, destructive |
| Tooltip | `tooltip.tsx` | Radix UI Tooltip primitive |
| Label | `label.tsx` | Radix UI Label primitive |
| ScrollArea | `scroll-area.tsx` | Radix UI ScrollArea primitive |
| Separator | `separator.tsx` | Radix UI Separator primitive |
| Textarea | `textarea.tsx` | Character-aware styling |
| Skeleton | `skeleton.tsx` | Loading placeholder |
| Sonner | `sonner.tsx` | Toast notifications |

## ANTI-PATTERNS

- **Manual Tailwind classes** - Use existing component props instead of adding custom classes
- **Hardcoded colors** - Use semantic tokens (primary, secondary, muted) from index.css, not raw hex/oklch values
- **Missing data-slot** - Always include data-slot attribute for parent styling hooks
- **Skipping dark mode** - Test both light and dark themes; use dark: prefix for dark-specific styles
- **Inline icons** - Use lucide-react icons, match existing size patterns (size-3, size-4, etc.)