// LedgerAlps — Composants UI réutilisables

import { type ReactNode } from 'react'
import { Loader2, AlertTriangle } from 'lucide-react'
import { cn } from '@/utils'

// ─── PageHeader ───────────────────────────────────────────────────────────────
interface PageHeaderProps {
  title: string
  subtitle?: string
  actions?: ReactNode
}

export function PageHeader({ title, subtitle, actions }: PageHeaderProps) {
  return (
    <div className="flex items-start justify-between mb-6">
      <div>
        <h1 className="text-2xl font-display font-700 text-alpine-900">{title}</h1>
        {subtitle && <p className="text-sm text-alpine-500 mt-1">{subtitle}</p>}
      </div>
      {actions && <div className="flex items-center gap-2">{actions}</div>}
    </div>
  )
}

// ─── LoadingSpinner ───────────────────────────────────────────────────────────
export function LoadingSpinner({ className }: { className?: string }) {
  return (
    <div className={cn('flex items-center justify-center py-12', className)}>
      <Loader2 className="w-6 h-6 text-alpine-400 animate-spin" />
    </div>
  )
}

// ─── EmptyState ───────────────────────────────────────────────────────────────
interface EmptyStateProps {
  icon?: ReactNode
  title: string
  description?: string
  action?: ReactNode
}

export function EmptyState({ icon, title, description, action }: EmptyStateProps) {
  return (
    <div className="flex flex-col items-center justify-center py-16 text-center">
      {icon && <div className="mb-4 text-alpine-300">{icon}</div>}
      <h3 className="text-base font-medium text-alpine-700">{title}</h3>
      {description && <p className="text-sm text-alpine-400 mt-1 max-w-xs">{description}</p>}
      {action && <div className="mt-4">{action}</div>}
    </div>
  )
}

// ─── StatCard ─────────────────────────────────────────────────────────────────
interface StatCardProps {
  label: string
  value: string
  delta?: string
  icon?: ReactNode
  accent?: boolean
}

export function StatCard({ label, value, delta, icon, accent }: StatCardProps) {
  return (
    <div className={cn('stat-card', accent && 'border-accent-200 bg-accent-50/30')}>
      <div className="flex items-start justify-between">
        <span className="stat-label">{label}</span>
        {icon && <span className="text-alpine-300">{icon}</span>}
      </div>
      <span className={cn('stat-value', accent && 'text-accent-700')}>{value}</span>
      {delta && <span className="stat-delta">{delta}</span>}
    </div>
  )
}

// ─── ErrorBanner ─────────────────────────────────────────────────────────────
export function ErrorBanner({ message }: { message: string }) {
  return (
    <div className="flex items-center gap-2 px-4 py-3 rounded-lg bg-danger-100
                    border border-danger-200 text-danger-700 text-sm mb-4">
      <AlertTriangle size={16} className="flex-shrink-0" />
      <span>{message}</span>
    </div>
  )
}

// ─── Badge status ─────────────────────────────────────────────────────────────
import { statusClass, statusLabel } from '@/utils'
import type { DocumentStatus } from '@/types'

export function StatusBadge({ status }: { status: DocumentStatus }) {
  return <span className={statusClass(status)}>{statusLabel(status)}</span>
}

// ─── Section titre ────────────────────────────────────────────────────────────
export function SectionTitle({ children }: { children: ReactNode }) {
  return (
    <h2 className="text-xs font-semibold text-alpine-500 uppercase tracking-wider mb-3">
      {children}
    </h2>
  )
}

export { PDFPreview } from './PDFPreview'
