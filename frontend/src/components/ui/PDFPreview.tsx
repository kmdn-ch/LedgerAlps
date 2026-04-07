// LedgerAlps — Aperçu PDF inline

import { useState, useEffect, useRef } from 'react'
import { Loader2, AlertTriangle, ExternalLink, X } from 'lucide-react'
import { cn } from '@/utils'

interface PDFPreviewProps {
  /** Fonction qui retourne le blob PDF (appelée uniquement si visible) */
  fetchPDF: () => Promise<Blob>
  filename?: string
  className?: string
  onClose?: () => void
}

export function PDFPreview({ fetchPDF, filename, className, onClose }: PDFPreviewProps) {
  const [blobUrl, setBlobUrl] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [error,   setError]   = useState<string | null>(null)
  const prevUrl = useRef<string | null>(null)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(null)

    fetchPDF()
      .then((blob) => {
        if (cancelled) return
        const url = URL.createObjectURL(blob)
        prevUrl.current = url
        setBlobUrl(url)
      })
      .catch(() => {
        if (!cancelled) setError("Impossible de charger le PDF.")
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })

    return () => {
      cancelled = true
      if (prevUrl.current) {
        URL.revokeObjectURL(prevUrl.current)
        prevUrl.current = null
      }
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  return (
    <div className={cn('relative flex flex-col rounded-lg border border-alpine-200 bg-white shadow-sm overflow-hidden', className)}>
      {/* Barre d'outils */}
      <div className="flex items-center justify-between px-3 py-2 border-b border-alpine-100 bg-alpine-50">
        <span className="text-xs font-medium text-alpine-600 truncate">
          {filename ?? 'Aperçu PDF'}
        </span>
        <div className="flex items-center gap-1 flex-shrink-0">
          {blobUrl && (
            <a
              href={blobUrl}
              target="_blank"
              rel="noreferrer"
              className="p-1 rounded hover:bg-alpine-200 text-alpine-500 hover:text-alpine-700 transition-colors"
              title="Ouvrir dans un nouvel onglet"
            >
              <ExternalLink size={14} />
            </a>
          )}
          {onClose && (
            <button
              onClick={onClose}
              className="p-1 rounded hover:bg-alpine-200 text-alpine-500 hover:text-alpine-700 transition-colors"
              title="Fermer l'aperçu"
            >
              <X size={14} />
            </button>
          )}
        </div>
      </div>

      {/* Contenu */}
      <div className="flex-1 min-h-0 relative" style={{ height: '600px' }}>
        {loading && (
          <div className="absolute inset-0 flex items-center justify-center bg-white">
            <Loader2 className="w-6 h-6 text-alpine-400 animate-spin" />
          </div>
        )}
        {error && (
          <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 text-danger-600">
            <AlertTriangle size={24} />
            <p className="text-sm">{error}</p>
          </div>
        )}
        {blobUrl && !loading && (
          <iframe
            src={blobUrl}
            title="Aperçu PDF"
            className="w-full h-full border-0"
          />
        )}
      </div>
    </div>
  )
}
