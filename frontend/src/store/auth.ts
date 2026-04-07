// LedgerAlps — Store d'authentification (Zustand + localStorage)

import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { User } from '@/types'

interface AuthState {
  user:         User | null
  accessToken:  string | null
  isAuth:       boolean
  setAuth:      (user: User, token: string) => void
  logout:       () => void
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      user:        null,
      accessToken: null,
      isAuth:      false,

      setAuth: (user, token) =>
        set({ user, accessToken: token, isAuth: true }),

      logout: () =>
        set({ user: null, accessToken: null, isAuth: false }),
    }),
    {
      name: 'ledgeralps-auth',
      partialize: (s) => ({ accessToken: s.accessToken, user: s.user, isAuth: s.isAuth }),
    }
  )
)
