// LedgerAlps — Restauration de la session au chargement
//
// Le jeton d'accès ne vit qu'en mémoire : un rechargement de page le perd. On demande donc au
// serveur d'en émettre un nouveau à partir du cookie HttpOnly, que le navigateur envoie tout
// seul. Sans cette étape, chaque F5 renverrait l'utilisateur à l'écran de connexion.
//
// Le hook est partagé par tous les écrans qui parlent au serveur (audit croisé Gemini, HIGH-20).
// Il vivait dans `RequireAuth` seul : les écrans hors de la coquille protégée — le changement du
// mot de passe temporaire, l'inscription du second facteur — n'avaient aucune restauration. Après
// un F5, « Changer mon mot de passe » partait sans jeton et répondait 401 ; fermer l'onglet et
// revenir donnait la même impasse, `mustChangePassword` étant persisté.

import { useEffect, useState } from 'react'
import { authApi } from '@/api/client'
import { useAuthStore } from '@/store/auth'

/** Vrai tant que la restauration est en cours. */
export function useRestoreSession(): boolean {
  // État initial calculé une seule fois : y a-t-il une session à restaurer ?
  const [restoring, setRestoring] = useState(() => {
    const s = useAuthStore.getState()
    return s.isAuth && !s.accessToken
  })

  // Tableau de dépendances vide, délibérément : l'effet ne doit s'exécuter qu'au montage. Avec
  // [isAuth, accessToken], poser le nouveau jeton déclenchait le nettoyage de l'effet AVANT que
  // le .finally ne s'exécute — « annulé » passait à vrai, setRestoring(false) était sauté, et
  // l'écran restait bloqué sur « Restauration de la session… » alors que l'appel avait pourtant
  // réussi.
  useEffect(() => {
    if (!restoring) return
    let monte = true

    authApi.refresh()
      .then((res) => {
        if (monte) useAuthStore.getState().setAccessToken(res.data.access_token)
      })
      .catch(() => {
        // Cookie absent, expiré ou révoqué : la session est bel et bien finie.
        if (monte) useAuthStore.getState().logout()
      })
      .finally(() => {
        if (monte) setRestoring(false)
      })

    return () => { monte = false }
  }, [])

  return restoring
}
