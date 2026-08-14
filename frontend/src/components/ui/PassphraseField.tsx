// Saisie d'une phrase de passe, avec sa robustesse évaluée pendant la frappe.
//
// LedgerAlps en manipule DEUX, et elles ne protègent pas la même chose. Les
// confondre est le risque principal de cet écran : quelqu'un qui croit avoir
// une seule phrase n'en note qu'une, et découvre le jour de la panne qu'il lui
// en fallait deux.
//
//   • Phrase des SAUVEGARDES — ouvre les fichiers .db.enc. C'est elle qui
//     compte quand la machine a disparu : les sauvegardes sont le seul chemin
//     de retour.
//   • Phrase de RÉCUPÉRATION de la base — ne sert qu'à retrouver la clé de la
//     base sur un autre compte Windows. Elle n'ouvre aucune sauvegarde.
//
// Elles doivent être différentes : une seule phrase compromise ouvrirait alors
// les deux, et il n'y a aucun bénéfice à les partager puisqu'on ne les tape
// presque jamais.
//
// La règle de robustesse est la même que celle du serveur
// (internal/db/passphrase.go : seize caractères, minuscule, majuscule, chiffre).
// Elle est reproduite ici pour être visible pendant la frappe plutôt que
// révélée par un refus après coup ; le serveur reste l'autorité, et un écart
// éventuel se solde par son message d'erreur, pas par une phrase faible acceptée.

import { useState, type ReactNode } from 'react'
import { useT } from '@/i18n/useT'
import type { Cle } from '@/i18n'
import { Check, Minus, Eye, EyeOff } from 'lucide-react'

export const PASSPHRASE_MIN_LEN = 16

// Le critère porte sa CLÉ : la liste est produite par une fonction pure,
// appelée aussi bien pendant le rendu que hors de tout composant
// (`passphraseIsStrong`), où `useT()` n'existe pas.
export interface PassphraseCheck { cle: Cle; met: boolean }

export function passphraseChecks(p: string): PassphraseCheck[] {
  return [
    { cle: 'pf.longueurMini', met: [...p].length >= PASSPHRASE_MIN_LEN },
    { cle: 'pf.uneMinuscule', met: /\p{Ll}/u.test(p) },
    { cle: 'pf.uneMajuscule', met: /\p{Lu}/u.test(p) },
    { cle: 'pf.unChiffre',    met: /\p{Nd}/u.test(p) },
  ]
}

export function passphraseIsStrong(p: string): boolean {
  return passphraseChecks(p).every(c => c.met)
}

// Le symbole et la longueur au-delà du minimum ne sont pas exigés mais
// renforcent : présentés comme un bonus, pas comme un obstacle de plus. Le but
// est d'encourager une phrase longue, pas de faire échouer la saisie.
function strengthOf(p: string): { score: number; cle: Cle | null; className: string } {
  if (p === '') return { score: 0, cle: null, className: '' }
  const met   = passphraseChecks(p).filter(c => c.met).length
  const bonus = /[^\p{L}\p{Nd}]/u.test(p) ? 1 : 0
  const long  = [...p].length >= 24 ? 1 : 0
  const score = met + bonus + long // 0..6

  if (met < 4)     return { score, cle: 'pf.insuffisante', className: 'bg-danger-500' }
  if (score >= 6)  return { score, cle: 'pf.excellente',   className: 'bg-success-700' }
  if (score === 5) return { score, cle: 'pf.solide',       className: 'bg-success-500' }
  return { score, cle: 'pf.acceptable', className: 'bg-warning-500' }
}

export function PassphraseStrength({ value }: { value: string }) {
  const t = useT()
  const checks = passphraseChecks(value)
  const { score, cle, className } = strengthOf(value)
  const allMet = checks.every(c => c.met)

  return (
    <div className="mt-2">
      <div className="flex items-center gap-2">
        <div className="flex-1 h-1.5 bg-alpine-100 rounded overflow-hidden">
          <div
            className={`h-full transition-all ${className}`}
            style={{ width: `${(score / 6) * 100}%` }}
          />
        </div>
        {cle && (
          <span className={`text-xs font-medium ${allMet ? 'text-success-700' : 'text-danger-700'}`}>
            {t(cle)}
          </span>
        )}
      </div>

      <ul className="mt-2 space-y-0.5">
        {checks.map(c => (
          <li key={c.cle} className={`text-xs flex items-center gap-1.5 ${
            c.met ? 'text-success-700' : 'text-alpine-500'
          }`}>
            {c.met ? <Check size={12} /> : <Minus size={12} />}
            {t(c.cle, { n: PASSPHRASE_MIN_LEN })}
          </li>
        ))}
      </ul>

      {/* Encourager plus long, sans en faire une condition : c'est la longueur
          qui décide face à une attaque hors ligne, pas la variété des
          caractères. Quatre mots sans rapport battent huit signes tortueux, et
          se retiennent. */}
      {allMet && score < 6 && (
        <p className="mt-1.5 text-xs text-alpine-500">
          {t('pf.faireMieux')}
        </p>
      )}
    </div>
  )
}

// PassphraseField réunit l'étiquette, la bascule d'affichage et la jauge.
//
// La bascule existe parce qu'une phrase qu'on ne relit pas se saisit de travers,
// et que l'erreur ne se découvre qu'à la restauration — c'est-à-dire au pire
// moment. Le risque de l'afficher est faible : on la saisit une fois, sur sa
// propre machine.
export function PassphraseField({
  id, label, value, onChange, hint, autoFocus, showStrength = true,
}: {
  id: string
  label: string
  value: string
  onChange: (v: string) => void
  hint?: ReactNode
  autoFocus?: boolean
  showStrength?: boolean
}) {
  const t = useT()
  const [visible, setVisible] = useState(false)

  return (
    <div>
      <label className="label" htmlFor={id}>{label}</label>
      {hint && <p className="text-xs text-alpine-600 mb-1.5">{hint}</p>}
      <div className="relative">
        <input
          id={id}
          type={visible ? 'text' : 'password'}
          value={value}
          onChange={e => onChange(e.target.value)}
          autoComplete="new-password"
          autoFocus={autoFocus}
          className="input w-full pr-10"
        />
        <button
          type="button"
          onClick={() => setVisible(!visible)}
          aria-label={visible ? t('sv.masquerPhrase') : t('sv.afficherPhrase')}
          className="absolute right-2 top-1/2 -translate-y-1/2 text-alpine-500 hover:text-alpine-900"
        >
          {visible ? <EyeOff size={16} /> : <Eye size={16} />}
        </button>
      </div>
      {showStrength && value !== '' && <PassphraseStrength value={value} />}
    </div>
  )
}
