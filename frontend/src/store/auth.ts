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
import type { User } from '@/types'

interface AuthState {
  user: User | null
  /** En mémoire uniquement : perdu au rechargement, restauré via le cookie. */
  accessToken: string | null
  isAuth: boolean
  /** Vrai tant que la tentative de restauration de session est en cours. */
  isRestoring: boolean
  setAuth: (user: User, accessToken: string) => void
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
      isRestoring: false,

      setAuth: (user, accessToken) =>
        set({ user, accessToken, isAuth: true, isRestoring: false }),

      setAccessToken: (token) => set({ accessToken: token }),

      setRestoring: (v) => set({ isRestoring: v }),

      logout: () =>
        set({ user: null, accessToken: null, isAuth: false, isRestoring: false }),
    }),
    {
      name: 'ledgeralps-auth',
      // accessToken est délibérément absent : c'est tout l'intérêt du changement.
      partialize: (s) => ({
        user: s.user,
        isAuth: s.isAuth,
      }),
    },
  ),
)
