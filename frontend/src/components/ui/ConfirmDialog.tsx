// LedgerAlps — Confirmation d'une action à conséquence
//
// « Êtes-vous sûr ? » n'aide personne à décider : la question ne dit ni ce qui
// va se passer, ni ce qui sera récupérable. Répétée, elle devient un réflexe et
// cesse de protéger — le clic sur « Oui » précède la lecture.
//
// Ce composant impose donc de fournir les CONSÉQUENCES, pas seulement un titre.
// Chaque appel énonce ce que l'action fait, ce qu'elle laisse intact, et si elle
// se défait. C'est la même exigence que pour les avis de conformité du produit :
// un avertissement n'a de valeur que s'il apprend quelque chose.

import { useEffect, useRef } from 'react'
import { AlertTriangle, Loader2 } from 'lucide-react'
import { useT } from '@/i18n/useT'

export interface ConfirmDialogProps {
  open: boolean
  title: string
  /** Ce que l'action fait, point par point. Le vide est un défaut d'appel. */
  consequences: React.ReactNode[]
  /** Précision facultative : ce qui n'est PAS touché, souvent le plus rassurant. */
  reassurance?: React.ReactNode
  confirmLabel: string
  /** Rouge pour ce qui ne se défait pas, neutre pour le reste. */
  tone?: 'danger' | 'normal'
  busy?: boolean
  onConfirm: () => void
  onCancel: () => void
}

export function ConfirmDialog({
  open, title, consequences, reassurance,
  confirmLabel, tone = 'normal', busy = false,
  onConfirm, onCancel,
}: ConfirmDialogProps) {
  const t = useT()
  const cancelRef = useRef<HTMLButtonElement>(null)

  // Le focus va sur « Annuler », pas sur la confirmation : une frappe sur
  // Entrée juste après l'ouverture ne doit pas valider l'action.
  useEffect(() => {
    if (open) cancelRef.current?.focus()
  }, [open])

  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onCancel() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open, onCancel])

  if (!open) return null

  const danger = tone === 'danger'

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-alpine-900/40 px-4"
      role="dialog"
      aria-modal="true"
      aria-labelledby="confirm-title"
      onClick={onCancel}
    >
      <div
        className="w-full max-w-lg rounded-lg bg-white shadow-lg"
        onClick={e => e.stopPropagation()}
      >
        <div className="px-5 py-4">
          <div className="flex items-start gap-3">
            <AlertTriangle
              size={20}
              className={`mt-0.5 flex-shrink-0 ${danger ? 'text-danger-700' : 'text-warning-700'}`}
            />
            <div className="flex-1">
              <h2 id="confirm-title" className="text-base font-medium">{title}</h2>

              <ul className="mt-2 space-y-1 text-sm text-alpine-700 list-disc list-inside">
                {consequences.map((c, i) => <li key={i}>{c}</li>)}
              </ul>

              {reassurance && (
                <p className="mt-2 text-sm text-alpine-600">{reassurance}</p>
              )}
            </div>
          </div>
        </div>

        <div className="flex justify-end gap-2 border-t border-neutral-200 px-5 py-3">
          <button ref={cancelRef} onClick={onCancel} className="btn-ghost btn-sm">
            {t('action.annuler')}
          </button>
          <button
            onClick={onConfirm}
            disabled={busy}
            className={`${danger ? 'btn-danger' : 'btn-primary'} btn-sm flex items-center gap-1.5`}
          >
            {busy && <Loader2 size={13} className="animate-spin" />}
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  )
}
