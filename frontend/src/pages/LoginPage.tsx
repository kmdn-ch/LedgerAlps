// LedgerAlps — Page de connexion

import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useNavigate } from 'react-router-dom'
import { Mountain, Eye, EyeOff } from 'lucide-react'
import { useState } from 'react'
import { authApi } from '@/api/client'
import { useAuthStore } from '@/store/auth'

const schema = z.object({
  email:    z.string().email('E-mail invalide'),
  password: z.string().min(8, 'Minimum 8 caractères'),
})
type FormData = z.infer<typeof schema>

export function LoginPage() {
  const navigate = useNavigate()
  const setAuth  = useAuthStore(s => s.setAuth)
  const [showPw, setShowPw]   = useState(false)
  const [error,  setError]    = useState('')
  const [loading, setLoading] = useState(false)

  const { register, handleSubmit, formState: { errors } } = useForm<FormData>({
    resolver: zodResolver(schema),
  })

  const onSubmit = async (data: FormData) => {
    setLoading(true)
    setError('')
    try {
      const res = await authApi.login(data.email, data.password)
      const user = { id: '', email: data.email, name: data.email.split('@')[0],
                     is_active: true, is_admin: false, created_at: '' }
      setAuth(user, res.data.access_token, res.data.refresh_token)
      navigate('/')
    } catch {
      setError('Identifiants incorrects.')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-alpine-950 p-4">
      {/* Background géométrique */}
      <div className="absolute inset-0 overflow-hidden pointer-events-none">
        <div className="absolute -top-32 -right-32 w-96 h-96 rounded-full
                        bg-accent-500/5 blur-3xl" />
        <div className="absolute -bottom-32 -left-32 w-96 h-96 rounded-full
                        bg-alpine-700/20 blur-3xl" />
      </div>

      <div className="relative w-full max-w-md">
        {/* Logo */}
        <div className="flex items-center justify-center gap-3 mb-8">
          <div className="w-10 h-10 rounded-xl bg-accent-500 flex items-center justify-center
                          shadow-lg shadow-accent-500/40">
            <Mountain size={20} className="text-white" />
          </div>
          <div>
            <div className="font-display font-700 text-xl text-white">LedgerAlps</div>
            <div className="text-xs text-alpine-400">Comptabilité suisse</div>
          </div>
        </div>

        {/* Card */}
        <div className="bg-alpine-900/80 border border-alpine-700/50 rounded-2xl
                        backdrop-blur-sm shadow-modal p-8">
          <h1 className="font-display font-700 text-lg text-white mb-1">Connexion</h1>
          <p className="text-sm text-alpine-400 mb-6">Accédez à votre espace comptable.</p>

          {error && (
            <div className="bg-danger-500/10 border border-danger-500/30 rounded-lg
                            px-4 py-3 text-danger-400 text-sm mb-4">
              {error}
            </div>
          )}

          <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
            <div>
              <label className="block text-xs font-medium text-alpine-400 mb-1.5 uppercase tracking-wide">
                Adresse e-mail
              </label>
              <input
                type="email"
                autoComplete="email"
                className={`w-full px-3 py-2.5 bg-alpine-800/80 border rounded-lg text-sm
                            text-white placeholder:text-alpine-500
                            focus:outline-none focus:ring-2 focus:ring-accent-500/50
                            ${errors.email ? 'border-danger-500' : 'border-alpine-700'}`}
                placeholder="vous@exemple.ch"
                {...register('email')}
              />
              {errors.email && <p className="text-xs text-danger-400 mt-1">{errors.email.message}</p>}
            </div>

            <div>
              <label className="block text-xs font-medium text-alpine-400 mb-1.5 uppercase tracking-wide">
                Mot de passe
              </label>
              <div className="relative">
                <input
                  type={showPw ? 'text' : 'password'}
                  autoComplete="current-password"
                  className={`w-full px-3 py-2.5 pr-10 bg-alpine-800/80 border rounded-lg text-sm
                              text-white placeholder:text-alpine-500
                              focus:outline-none focus:ring-2 focus:ring-accent-500/50
                              ${errors.password ? 'border-danger-500' : 'border-alpine-700'}`}
                  {...register('password')}
                />
                <button
                  type="button"
                  onClick={() => setShowPw(!showPw)}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-alpine-500
                             hover:text-alpine-300 transition-colors"
                >
                  {showPw ? <EyeOff size={15} /> : <Eye size={15} />}
                </button>
              </div>
              {errors.password && <p className="text-xs text-danger-400 mt-1">{errors.password.message}</p>}
            </div>

            <button
              type="submit"
              disabled={loading}
              className="w-full py-2.5 bg-accent-500 hover:bg-accent-600 text-white font-medium
                         rounded-lg text-sm transition-all duration-150 active:scale-[0.98]
                         disabled:opacity-50 disabled:cursor-not-allowed mt-2"
            >
              {loading ? 'Connexion…' : 'Se connecter'}
            </button>
          </form>
        </div>

        <p className="text-center text-xs text-alpine-600 mt-6">
          LedgerAlps — Données locales · CO · nLPD
        </p>
      </div>
    </div>
  )
}
