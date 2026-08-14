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

import { useEffect, useCallback, useRef } from 'react'
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
  // # Le garde doit se taire quand la saisie a été ENREGISTRÉE
  //
  // Sans cela il se retourne contre ce qu'il protège : « Créer en brouillon »
  // enregistre la facture, puis navigue vers elle — et le garde, qui ne voit
  // qu'une navigation sur un formulaire encore « dirty », affiche « Quitter
  // cette page ? Ce que vous avez saisi sera perdu ». Le message est faux, la
  // facture existe ; et l'utilisateur qui le croit clique « Annuler » et reste
  // bloqué sur un écran dont le bouton principal ne fait plus rien.
  //
  // Un `ref` et non un état : `désarmer()` est appelé juste avant `navigate()`,
  // dans le même tour. Un état ne serait pas encore appliqué quand React Router
  // consulte la condition, et le garde se déclencherait quand même.
  const désarmé = useRef(false)

  useBeforeUnload(dirty && !désarmé.current)

  const blocker = useBlocker(
    ({ currentLocation, nextLocation }) =>
      dirty && !désarmé.current && currentLocation.pathname !== nextLocation.pathname,
  )

  const confirmLeave = useCallback(() => {
    if (blocker.state === 'blocked') blocker.proceed()
  }, [blocker])

  const cancelLeave = useCallback(() => {
    if (blocker.state === 'blocked') blocker.reset()
  }, [blocker])

  /** À appeler avant de naviguer après un enregistrement réussi. */
  const désarmer = useCallback(() => {
    désarmé.current = true
  }, [])

  return {
    /** Vrai quand une navigation attend la décision de l'utilisateur. */
    blocked: blocker.state === 'blocked',
    confirmLeave,
    cancelLeave,
    désarmer,
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
