// Les conseils de sécurité de l'écran de connexion, frappés à la machine.
//
// # Pourquoi un conseil, et pourquoi ici
//
// L'écran de connexion est le seul moment de la journée où l'utilisateur n'a
// rien à faire d'autre que regarder. Une consigne de sécurité lue là a une
// chance d'être lue ; la même dans un manuel n'en a aucune.
//
// # Le tirage
//
// Un conseil est tiré au sort à chaque ouverture, puis les suivants s'enchaînent.
// Le tirage évite CELUI DE LA DERNIÈRE FOIS, retenu dans le navigateur : sans
// cette précaution, une machine ouverte deux fois de suite a une chance sur
// trente-deux de réafficher le même conseil, ce qui donne l'impression d'une
// liste figée. Aucune autre mémoire n'est tenue — pas de « déjà vus » à
// accumuler pour trente-deux phrases.
//
// # La machine à écrire, et qui n'en veut pas
//
// L'effet respecte `prefers-reduced-motion` : quelqu'un qui a demandé à son
// système de calmer les animations reçoit la phrase entière, d'un coup. Une
// animation qu'on ne peut pas arrêter n'est pas un détail de goût — elle rend
// certaines pages illisibles.

import { useEffect, useRef, useState } from 'react'
import { ShieldCheck } from 'lucide-react'
import { useT } from '@/i18n/useT'
import type { Cle } from '@/i18n'

const NOMBRE_CONSEILS = 32
const MEMOIRE = 'ledgeralps-dernier-conseil'

/** Les clés `conseil.01` … `conseil.32`, dans l'ordre du catalogue. */
const CLES: Cle[] = Array.from({ length: NOMBRE_CONSEILS }, (_, i) =>
  `conseil.${String(i + 1).padStart(2, '0')}` as Cle)

const VITESSE_FRAPPE = 28   // ms par caractère
const TEMPS_LECTURE  = 9000 // ms pendant lesquels la phrase reste entière

/** Un tirage uniforme qui évite l'indice `sauf`. */
function tirer(sauf: number): number {
  if (NOMBRE_CONSEILS <= 1) return 0
  const i = Math.floor(Math.random() * (NOMBRE_CONSEILS - 1))
  return i >= sauf ? i + 1 : i
}

function reduireLesAnimations(): boolean {
  return typeof window !== 'undefined'
    && window.matchMedia?.('(prefers-reduced-motion: reduce)').matches === true
}

export function ConseilSecurite({ className = '' }: { className?: string }) {
  const t = useT()

  // Le premier conseil est choisi UNE fois, au montage. Le mettre dans l'état
  // initial plutôt que dans un effet évite d'afficher une phrase puis une
  // autre au premier rendu.
  const [indice, setIndice] = useState(() => {
    const dernier = Number(localStorage.getItem(MEMOIRE))
    return tirer(Number.isInteger(dernier) && dernier >= 0 && dernier < NOMBRE_CONSEILS
      ? dernier : -1)
  })
  const [ecrit, setEcrit] = useState('')
  const minuteries = useRef<number[]>([])

  const phrase = t(CLES[indice])

  useEffect(() => {
    localStorage.setItem(MEMOIRE, String(indice))
  }, [indice])

  useEffect(() => {
    // Toute minuterie du conseil précédent est annulée : sans cela, changer de
    // langue en pleine frappe ferait courir deux machines à écrire sur la même
    // ligne, une lettre sur deux venant de chaque phrase.
    minuteries.current.forEach(clearTimeout)
    minuteries.current = []

    if (reduireLesAnimations()) {
      setEcrit(phrase)
      minuteries.current.push(window.setTimeout(
        () => setIndice(i => tirer(i)), TEMPS_LECTURE))
      return
    }

    setEcrit('')
    for (let i = 1; i <= phrase.length; i++) {
      minuteries.current.push(window.setTimeout(
        () => setEcrit(phrase.slice(0, i)), i * VITESSE_FRAPPE))
    }
    minuteries.current.push(window.setTimeout(
      () => setIndice(i => tirer(i)),
      phrase.length * VITESSE_FRAPPE + TEMPS_LECTURE))

    return () => {
      minuteries.current.forEach(clearTimeout)
      minuteries.current = []
    }
  }, [phrase])

  const fini = ecrit.length === phrase.length

  return (
    <div className={className}>
      <div className="flex items-center gap-1.5 mb-2">
        <ShieldCheck size={12} className="text-brand-orange shrink-0" aria-hidden="true" />
        <span className="text-[10px] font-semibold uppercase tracking-wider text-slate-500">
          {t('conseil.titre')}
        </span>
      </div>
      {/* `aria-live` en `polite` et non `assertive` : un lecteur d'écran
          annonce la phrase quand il a fini ce qu'il disait, sans couper la
          lecture d'un champ du formulaire.

          La phrase COMPLÈTE est donnée à l'assistance technique, pas la
          version en cours de frappe — sinon elle serait relue lettre après
          lettre, trente fois de suite. */}
      <p className="sr-only" aria-live="polite">{phrase}</p>
      {/* Le double de la taille précédente, en gras : le conseil n'est plus un
          bas de page, c'est ce que le panneau donne à lire.

          `min-h` réserve la hauteur du plus long des trente-deux conseils. Sans
          elle, le panneau se détend et se contracte à chaque phrase, et tout
          ce qui est en dessous — les deux garanties — sautille pendant la
          frappe.

          La réserve est calée sur l'ALLEMAND, mesuré dans le navigateur à
          165 px là où le français plafonne à 132 : les mots composés y passent
          rarement à la ligne au même endroit, et une réserve taillée pour le
          français aurait sauté sur deux conseils allemands. */}
      <p aria-hidden="true"
         className="text-2xl font-bold text-slate-300 leading-snug min-h-[11rem]">
        {ecrit}
        {!fini && <span className="inline-block w-[3px] h-[0.9em] -mb-[0.05em]
                                   ml-[2px] bg-brand-orange animate-pulse" />}
      </p>
    </div>
  )
}
