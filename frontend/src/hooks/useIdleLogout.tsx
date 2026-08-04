// Déconnexion après inactivité.
//
// Le point délicat n'est pas de compter le temps, c'est de ne pas faire perdre
// de travail. LedgerAlps n'enregistre aucun brouillon automatique : une
// déconnexion au milieu d'une facture de quinze lignes efface les quinze
// lignes. Un réglage qui coûte ça finit désactivé — et une session qui ne se
// ferme jamais est pire que celle qui se fermait trop tôt.
//
// D'où trois choix :
//
//  1. L'activité comptée est celle de l'utilisateur, pas celle du réseau. Une
//     page qui recharge des données toutes les trente secondes ne doit pas
//     maintenir la session de quelqu'un parti déjeuner.
//  2. Un avertissement compté à rebours avant la coupure, avec un bouton pour
//     rester. Personne ne doit être déconnecté sans l'avoir vu venir.
//  3. Le délai vient du serveur et peut être coupé. C'est un réglage, pas une
//     conviction.
//
// L'onglet caché ne suspend pas le compte : quelqu'un qui bascule sur son
// navigateur web et laisse LedgerAlps derrière est précisément le cas visé.

import { useEffect, useRef, useState, useCallback } from 'react'

// Combien de temps l'avertissement reste affiché avant la déconnexion.
// Soixante secondes : assez pour revenir d'une autre fenêtre et cliquer,
// assez court pour que la protection garde un sens.
const WARNING_SECONDS = 60

// Les événements qui comptent comme « quelqu'un est là ». Volontairement des
// gestes humains : un rafraîchissement de données en arrière-plan n'en est pas
// un.
const ACTIVITY_EVENTS = [
  'mousedown', 'mousemove', 'keydown', 'wheel', 'touchstart', 'scroll',
] as const

export interface IdleLogoutState {
  /** Vrai quand le décompte final est affiché. */
  warning: boolean
  /** Secondes restantes avant la déconnexion, pendant l'avertissement. */
  remaining: number
  /** Repousse la déconnexion et referme l'avertissement. */
  stay: () => void
}

/**
 * useIdleLogout appelle `onTimeout` après `minutes` sans activité.
 * `minutes` à 0 (ou négatif) désactive complètement le mécanisme.
 */
export function useIdleLogout(minutes: number, onTimeout: () => void): IdleLogoutState {
  const [warning, setWarning]     = useState(false)
  const [remaining, setRemaining] = useState(WARNING_SECONDS)

  // Les minuteries vivent dans des refs : les remettre à zéro ne doit pas
  // provoquer de rendu, sans quoi chaque mouvement de souris redessinerait
  // l'application entière.
  const idleTimer  = useRef<number | undefined>(undefined)
  const countdown  = useRef<number | undefined>(undefined)
  // onTimeout passe par une ref pour que le rappel puisse changer sans
  // relancer les écouteurs — les réattacher à chaque rendu remettrait le
  // compteur à zéro en boucle, et la déconnexion n'arriverait jamais.
  const onTimeoutRef = useRef(onTimeout)
  useEffect(() => { onTimeoutRef.current = onTimeout }, [onTimeout])

  const clearAll = useCallback(() => {
    if (idleTimer.current !== undefined) window.clearTimeout(idleTimer.current)
    if (countdown.current !== undefined) window.clearInterval(countdown.current)
    idleTimer.current = undefined
    countdown.current = undefined
  }, [])

  const startCountdown = useCallback(() => {
    setWarning(true)
    setRemaining(WARNING_SECONDS)
    let left = WARNING_SECONDS
    countdown.current = window.setInterval(() => {
      left -= 1
      setRemaining(left)
      if (left <= 0) {
        clearAll()
        setWarning(false)
        onTimeoutRef.current()
      }
    }, 1000)
  }, [clearAll])

  const reset = useCallback(() => {
    clearAll()
    setWarning(false)
    if (minutes <= 0) return
    // L'avertissement occupe les dernières secondes du délai, il ne s'y ajoute
    // pas : régler « 10 minutes » doit déconnecter à 10 minutes, pas à 11.
    const idleMs = Math.max(minutes * 60_000 - WARNING_SECONDS * 1000, 1000)
    idleTimer.current = window.setTimeout(startCountdown, idleMs)
  }, [minutes, clearAll, startCountdown])

  useEffect(() => {
    if (minutes <= 0) {
      clearAll()
      setWarning(false)
      return
    }
    // Pendant l'avertissement, un mouvement de souris ne doit PAS annuler tout
    // seul : sinon un chat sur le clavier — ou un écran tactile dans une poche —
    // maintiendrait la session ouverte indéfiniment. Il faut cliquer.
    const onActivity = () => { if (!warningRef.current) reset() }
    ACTIVITY_EVENTS.forEach(e =>
      window.addEventListener(e, onActivity, { passive: true }))
    reset()
    return () => {
      ACTIVITY_EVENTS.forEach(e => window.removeEventListener(e, onActivity))
      clearAll()
    }
  }, [minutes, reset, clearAll])

  // Miroir de `warning` lisible depuis l'écouteur sans le réattacher.
  const warningRef = useRef(false)
  useEffect(() => { warningRef.current = warning }, [warning])

  return { warning, remaining, stay: reset }
}
