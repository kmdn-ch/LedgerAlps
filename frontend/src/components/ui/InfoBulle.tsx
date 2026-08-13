// Le petit « i » qui explique à quoi sert un écran.
//
// # Pourquoi une bulle plutôt qu'une ligne sous le titre
//
// Une phrase d'explication sous chaque titre aide le premier jour et encombre
// les mille suivants : elle prend une ligne sur tous les écrans, pour tout le
// monde, y compris ceux qui n'ont plus rien à apprendre. La bulle met la même
// phrase à un survol de distance et ne coûte rien à qui n'en a pas besoin.
//
// # Les deux gestes, et pourquoi les deux
//
// Le SURVOL ouvre — c'est le geste qu'on essaie d'instinct, et il ne demande
// aucune décision. Le CLIC épingle : la bulle reste, on peut y lire une adresse
// d'écran sans que le texte s'évapore parce que la souris a bougé de trois
// pixels. Épinglée, elle se ferme par un second clic, par Échap, ou par un clic
// ailleurs.
//
// Le clic n'est pas un luxe : au doigt, il n'y a pas de survol, et au clavier
// non plus. Une aide qui n'existerait qu'au survol n'existerait pas pour eux.

import { useEffect, useId, useRef, useState, type ReactNode } from 'react'
import { Info } from 'lucide-react'
import { useT } from '@/i18n/useT'

interface InfoBulleProps {
  /** Le texte de l'aide. Une à trois phrases : au-delà, c'est de la doc. */
  children: ReactNode
  /** Où la bulle se déplie. `right` quand le titre est collé au bord gauche. */
  cote?: 'right' | 'left'
}

export function InfoBulle({ children, cote = 'right' }: InfoBulleProps) {
  const t = useT()
  const id = useId()
  const [survole, setSurvole] = useState(false)
  const [epingle, setEpingle] = useState(false)
  const conteneur = useRef<HTMLSpanElement>(null)

  const ouverte = survole || epingle

  // Épinglée, la bulle se ferme comme tout ce qui flotte : Échap, ou un clic
  // ailleurs. Les écouteurs ne vivent QUE pendant l'épinglage — en poser sur
  // le document pour chaque « i » d'une page en placerait une dizaine qui ne
  // servent à rien.
  useEffect(() => {
    if (!epingle) return
    function surTouche(e: KeyboardEvent) {
      if (e.key === 'Escape') setEpingle(false)
    }
    function surClic(e: MouseEvent) {
      if (!conteneur.current?.contains(e.target as Node)) setEpingle(false)
    }
    document.addEventListener('keydown', surTouche)
    document.addEventListener('mousedown', surClic)
    return () => {
      document.removeEventListener('keydown', surTouche)
      document.removeEventListener('mousedown', surClic)
    }
  }, [epingle])

  return (
    <span
      ref={conteneur}
      className="relative inline-flex align-middle"
      onMouseEnter={() => setSurvole(true)}
      onMouseLeave={() => setSurvole(false)}
    >
      <button
        type="button"
        aria-label={t('aide.aQuoiSertCetEcran')}
        aria-expanded={ouverte}
        aria-describedby={ouverte ? id : undefined}
        onClick={() => setEpingle(v => !v)}
        onFocus={() => setSurvole(true)}
        onBlur={() => setSurvole(false)}
        className="text-alpine-400 hover:text-accent-600 focus:text-accent-600
                   focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-500
                   rounded-full transition-colors"
      >
        <Info size={15} />
      </button>

      {ouverte && (
        <span
          id={id}
          role="tooltip"
          className={`absolute top-full mt-2 z-30 w-80 max-w-[80vw] rounded-md
                      border border-alpine-200 bg-white px-3 py-2 text-xs
                      font-normal leading-relaxed text-alpine-700 shadow-lg
                      ${cote === 'left' ? 'right-0' : 'left-0'}`}
        >
          {children}
        </span>
      )}
    </span>
  )
}
