// Protéger une saisie en cours.
//
// LedgerAlps n'enregistre aucun brouillon automatique. Une facture de quinze
// lignes disparaît donc entièrement sur un clic à côté — et le clic à côté est
// facile : le voile d'une fenêtre modale ferme au moindre contact, un lien du
// menu emmène ailleurs, un raccourci recharge la page.
//
// Trois sorties, trois protections. Elles ne se remplacent pas :
//
//   1. Le voile et la croix d'une fenêtre modale — traité par askBeforeClose.
//   2. Un lien interne — traité par le blocage de navigation de React Router.
//   3. La fermeture de l'onglet ou un rechargement — traité par beforeunload,
//      seul événement que le navigateur accepte d'intercepter, et dont il
//      impose le libellé.
//
// Le garde ne s'active que si quelque chose a été saisi. Demander confirmation
// pour un formulaire vide serait le meilleur moyen d'apprendre à cliquer
// « Quitter » sans lire.

import { useEffect, useCallback } from 'react'
import { useBlocker } from 'react-router-dom'

/**
 * useBeforeUnload avertit avant la fermeture de l'onglet ou un rechargement.
 *
 * Le navigateur impose son propre libellé : aucun texte fourni ici ne s'affiche.
 * C'est une protection contre l'accident, pas un endroit où expliquer.
 */
export function useBeforeUnload(active: boolean) {
  useEffect(() => {
    if (!active) return
    const handler = (e: BeforeUnloadEvent) => {
      e.preventDefault()
      // returnValue reste requis par plusieurs navigateurs, même si sa valeur
      // est ignorée depuis longtemps.
      e.returnValue = ''
    }
    window.addEventListener('beforeunload', handler)
    return () => window.removeEventListener('beforeunload', handler)
  }, [active])
}

/**
 * useUnsavedGuard bloque la navigation interne et la fermeture de l'onglet tant
 * que `dirty` est vrai.
 *
 * Rend l'état du blocage pour que l'appelant affiche sa propre confirmation —
 * une boîte native ne dirait ni ce qui sera perdu, ni ce qui a déjà été
 * enregistré.
 */
export function useUnsavedGuard(dirty: boolean) {
  useBeforeUnload(dirty)

  const blocker = useBlocker(
    ({ currentLocation, nextLocation }) =>
      dirty && currentLocation.pathname !== nextLocation.pathname,
  )

  const confirmLeave = useCallback(() => {
    if (blocker.state === 'blocked') blocker.proceed()
  }, [blocker])

  const cancelLeave = useCallback(() => {
    if (blocker.state === 'blocked') blocker.reset()
  }, [blocker])

  return {
    /** Vrai quand une navigation attend la décision de l'utilisateur. */
    blocked: blocker.state === 'blocked',
    confirmLeave,
    cancelLeave,
  }
}

/**
 * askBeforeClose enveloppe la fermeture d'une fenêtre modale.
 *
 * Le voile d'une modale ferme au moindre clic : c'est commode pour consulter,
 * destructeur pour une saisie. La fonction rendue ne demande rien quand rien
 * n'a été saisi.
 */
export function askBeforeClose(dirty: boolean, close: () => void, ask: () => void) {
  return () => (dirty ? ask() : close())
}
