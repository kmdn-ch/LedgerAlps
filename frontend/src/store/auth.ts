// LedgerAlps — Store d'authentification (Zustand + localStorage)

import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { User } from '@/types'

interface AuthState {
  user:          User | null
  accessToken:   string | null
  refreshToken:  string | null
  isAuth:        boolean
  setAuth:       (user: User, accessToken: string, refreshToken?: string) => void
  setAccessToken:(token: string) => void
  logout:        () => void
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      user:         null,
      accessToken:  null,
      refreshToken: null,
      isAuth:       false,

      setAuth: (user, accessToken, refreshToken) =>
        set({ user, accessToken, refreshToken: refreshToken ?? null, isAuth: true }),

      setAccessToken: (token) =>
        set({ accessToken: token }),

      logout: () =>
        set({ user: null, accessToken: null, refreshToken: null, isAuth: false }),
    }),
    {
      name: 'ledgeralps-auth',
      partialize: (s) => ({
        accessToken:  s.accessToken,
        refreshToken: s.refreshToken,
        user:         s.user,
        isAuth:       s.isAuth,
      }),
    }
  )
)
