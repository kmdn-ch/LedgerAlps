// LedgerAlps — Store d'authentification
//
// Le jeton de rafraîchissement n'est plus ici : il vit dans un cookie HttpOnly
// que le navigateur envoie tout seul aux routes d'authentification, et qu'aucun
// script ne peut lire. Le jeton d'accès reste en mémoire uniquement — jamais
// dans localStorage — ce qui fait qu'une injection de script ne peut au pire
// dérober qu'un jeton expirant dans l'heure, au lieu de trente jours d'accès.
//
// Seuls l'identité affichée et l'indicateur de session sont persistés : ce ne
// sont pas des identifiants, et les conserver évite un écran de connexion qui
// clignote à chaque rechargement pendant que le rafraîchissement s'effectue.

import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { User, UserRole } from '@/types'

interface AuthState {
  user: User | null
  /** En mémoire uniquement : perdu au rechargement, restauré via le cookie. */
  accessToken: string | null
  isAuth: boolean
  // Le rôle sert UNIQUEMENT à l'affichage : ne pas proposer un écran qui
  // répondra 403, et dire à l'utilisateur avec quel compte il travaille. Le
  // serveur relit le rôle dans la base à chaque requête et ne fait aucune
  // confiance à cette valeur — la modifier dans le navigateur ne donne aucun
  // droit.
  role: UserRole | null
  // Vrai tant que le mot de passe temporaire n'a pas été remplacé. Sert à
  // router vers l'écran de changement ; le serveur refuse de toute façon toute
  // requête d'un tel compte, cette valeur ne donne donc aucun accès.
  mustChangePassword: boolean
  // Vrai quand un compte administrateur n'a pas encore inscrit de second
  // facteur. Même statut que ci-dessus : sert à router, ne donne aucun accès —
  // le serveur refuse toute requête d'un administrateur non inscrit.
  mfaEnrolmentRequired: boolean
  /** Vrai tant que la tentative de restauration de session est en cours. */
  isRestoring: boolean
  setAuth: (user: User, accessToken: string, role?: UserRole | null,
            mustChange?: boolean, mfaEnrolment?: boolean) => void
  clearMustChange: () => void
  clearMfaEnrolment: () => void
  setAccessToken: (token: string) => void
  setRestoring: (v: boolean) => void
  logout: () => void
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      user: null,
      accessToken: null,
      isAuth: false,
      role: null,
      mustChangePassword: false,
      mfaEnrolmentRequired: false,
      isRestoring: false,

      setAuth: (user, accessToken, role = null, mustChange = false, mfaEnrolment = false) =>
        set({ user, accessToken, isAuth: true, isRestoring: false, role,
              mustChangePassword: mustChange, mfaEnrolmentRequired: mfaEnrolment }),

      clearMustChange: () => set({ mustChangePassword: false }),

      clearMfaEnrolment: () => set({ mfaEnrolmentRequired: false }),

      setAccessToken: (token) => set({ accessToken: token }),

      setRestoring: (v) => set({ isRestoring: v }),

      logout: () =>
        set({ user: null, accessToken: null, isAuth: false, isRestoring: false,
              role: null, mustChangePassword: false, mfaEnrolmentRequired: false }),
    }),
    {
      name: 'ledgeralps-auth',
      // accessToken est délibérément absent : c'est tout l'intérêt du changement.
      partialize: (s) => ({
        user: s.user,
        isAuth: s.isAuth,
        role: s.role,
        mustChangePassword: s.mustChangePassword,
        mfaEnrolmentRequired: s.mfaEnrolmentRequired,
      }),
    },
  ),
)
