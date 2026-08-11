// LedgerAlps — Page de connexion

import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { Mountain, Eye, EyeOff, Smartphone, ArrowLeft, LifeBuoy } from 'lucide-react'
import { useState } from 'react'
import { authApi } from '@/api/client'
import { useAuthStore } from '@/store/auth'
import { useT, useTv } from '@/i18n/useT'

const schema = z.object({
  email:    z.string().email('val.emailInvalide'),
  password: z.string().min(8, 'cx.motDePasseCourt'),
})
type FormData = z.infer<typeof schema>

export function LoginPage() {
  const t = useT()
  const tv = useTv()
  // Une déconnexion automatique doit se dire. Renvoyer quelqu'un sur l'écran de
  // connexion sans explication ressemble à une panne, et c'est la première
  // chose qu'on signale au support.
  const [params] = useSearchParams()
  const idle = params.get('raison') === 'inactivite'

  const navigate = useNavigate()
  const setAuth  = useAuthStore(s => s.setAuth)
  const [showPw, setShowPw]   = useState(false)
  const [error,  setError]    = useState('')
  const [loading, setLoading] = useState(false)

  // Deuxième étape. Le jeton d'attente ne vit qu'ici, en mémoire, et cinq
  // minutes au plus : il ne vaut que pour l'échange contre une vraie session,
  // et le serveur le refuse sur toute autre route.
  const [challenge, setChallenge] = useState<{ token: string; email: string } | null>(null)
  const [code, setCode] = useState('')

  // Le code de secours a son propre mode, et pas seulement sa propre phrase.
  //
  // Le champ acceptait six chiffres — plafonné à sept caractères, clavier
  // numérique, espacement de chiffres — pendant qu'une ligne de texte invitait à
  // y saisir un code de secours de onze caractères. Il était donc littéralement
  // impossible à taper : la moitié se perdait au sixième caractère. Une
  // consigne qui décrit un mécanisme absent est pire qu'aucune consigne, et
  // c'est exactement le moment où l'on a le moins envie de chercher.
  //
  // Un mode explicite, avec son bouton : le champ change de forme, de clavier et
  // de longueur, et l'utilisateur voit qu'il est au bon endroit.
  const [mode, setMode] = useState<'totp' | 'recovery'>('totp')
  // Décoché par défaut, toujours. Une protection qu'on lève sans le savoir
  // n'en est plus une : la dispense doit être un geste conscient.
  const [remember, setRemember] = useState(false)
  const secours = mode === 'recovery'

  // La saisie est normalisée comme le serveur la normalise : majuscules, sans
  // tiret ni espace. Le papier se recopie rarement au caractère près.
  const codeUtile = secours
    ? code.toUpperCase().replace(/[\s-]/g, '')
    : code.replace(/\s/g, '')
  const codeComplet = secours ? codeUtile.length >= 10 : codeUtile.length >= 6

  // Poser la session, quel que soit le chemin qui y a mené. Une seule fonction :
  // deux copies auraient divergé, et celle qui aurait oublié une règle serait
  // justement celle empruntée après avoir prouvé son identité deux fois.
  const enter = (email: string, data: {
    access_token: string; role?: string | null
    must_change_password?: boolean; mfa_enrolment_required?: boolean
  }) => {
    const user = { id: '', email, name: email.split('@')[0],
                   is_active: true, is_admin: false, created_at: '' }
    setAuth(user, data.access_token, (data.role ?? null) as never,
            data.must_change_password === true,
            data.mfa_enrolment_required === true)
    if (data.must_change_password === true) { navigate('/change-password'); return }
    if (data.mfa_enrolment_required === true) { navigate('/second-facteur'); return }
    navigate('/')
  }

  const submitCode = async () => {
    if (!challenge) return
    setLoading(true)
    setError('')
    try {
      const res = await authApi.mfaVerify(challenge.token, codeUtile, remember)
      enter(challenge.email, res.data)
    } catch (e) {
      const status = (e as { response?: { status?: number } }).response?.status
      if (status === 401) {
        // Le conseil doit correspondre à ce qui a été tenté. Parler d'horloge
        // de téléphone à quelqu'un qui saisit un code de secours l'envoie
        // chercher au mauvais endroit.
        setError(t(secours ? 'cx.secoursRefuse' : 'cx.codeIncorrect'))
      } else if (status === 429) {
        setError(t('cx.tropDeTentatives'))
      } else {
        setError(t('cx.verificationEchouee'))
        setChallenge(null)
      }
      setCode('')
    } finally {
      setLoading(false)
    }
  }

  const { register, handleSubmit, formState: { errors } } = useForm<FormData>({
    resolver: zodResolver(schema),
  })

  const onSubmit = async (data: FormData) => {
    setLoading(true)
    setError('')
    try {
      const res = await authApi.login(data.email, data.password)
      // Le mot de passe est juste, mais il ne suffit pas : rien n'a encore été
      // délivré qu'un jeton d'attente.
      if (res.data.mfa_required === true) {
        setChallenge({ token: res.data.mfa_token, email: data.email })
        return
      }
      enter(data.email, res.data)
    } catch {
      setError(t('connexion.identifiantsIncorrects'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-alpine-950 p-4">
      <div className="relative w-full max-w-md">
        {idle && (
          <div className="mb-5 rounded-md border border-warning-500 bg-warning-100 px-4 py-3 text-sm">
            <p className="font-medium">{t('connexion.sessionFermee')}</p>
            <p className="mt-1 text-alpine-700">
              {t('cx.deconnexionAuto')}
            </p>
          </div>
        )}

        {/* Logo */}
        <div className="flex items-center justify-center gap-3 mb-8">
          <div className="w-10 h-10 rounded-xl bg-accent-500 flex items-center justify-center
                          shadow-lg shadow-accent-500/40">
            <Mountain size={20} className="text-white" />
          </div>
          <div>
            <div className="font-display font-700 text-xl text-white">LedgerAlps</div>
          </div>
        </div>

        {/* Card */}
        <div className="bg-alpine-900/80 border border-alpine-700/50 rounded-2xl
                        backdrop-blur-sm shadow-modal p-8">
          <h1 className="font-display font-700 text-lg text-white mb-1">
            {challenge ? t('connexion.verification') : t('connexion.titre')}
          </h1>
          <p className="text-sm text-alpine-400 mb-6">
            {!challenge
              ? t('connexion.sousTitre')
              : secours
                ? t('connexion.saisirCodeSecours')
                : t('connexion.saisirCodeApp')}
          </p>

          {error && (
            <div className="bg-danger-500/10 border border-danger-500/30 rounded-lg
                            px-4 py-3 text-danger-500 text-sm mb-4">
              {error}
            </div>
          )}

          {/* Deuxième étape : le mot de passe est accepté, la session n'existe
              pas encore. Rien de ce qui s'affiche ici n'ouvre quoi que ce soit
              tant que le code n'est pas validé par le serveur. */}
          {challenge ? (
            <div className="space-y-4">
              <div className="flex items-start gap-2.5 text-sm text-alpine-300">
                {secours
                  ? <LifeBuoy size={16} className="text-accent-500 mt-0.5 shrink-0" />
                  : <Smartphone size={16} className="text-accent-500 mt-0.5 shrink-0" />}
                <span>
                  {secours
                    ? <>{t('connexion.aideCodeSecours')}<strong className="text-white">{challenge.email}</strong>.</>
                    : <>{t('connexion.aideCodeApp')}<strong className="text-white">{challenge.email}</strong>.</>}
                </span>
              </div>

              <div>
                <label htmlFor="otp"
                       className="block text-xs font-medium text-alpine-400 mb-1.5 uppercase tracking-wide">
                  {t(secours ? 'cx.champCodeSecours' : 'cx.champCodeSixChiffres')}
                </label>
                {/* Le champ change vraiment de nature selon le mode : longueur,
                    clavier, casse, espacement. C'est ce qui manquait — la
                    consigne parlait de codes de secours pendant que le champ
                    n'acceptait que six chiffres. */}
                <input
                  id="otp"
                  key={mode}
                  autoFocus
                  inputMode={secours ? 'text' : 'numeric'}
                  autoComplete={secours ? 'off' : 'one-time-code'}
                  autoCapitalize="characters"
                  spellCheck={false}
                  maxLength={secours ? 13 : 7}
                  value={code}
                  onChange={e => setCode(secours ? e.target.value.toUpperCase() : e.target.value)}
                  onKeyDown={e => { if (e.key === 'Enter' && codeComplet) submitCode() }}
                  className={`w-full px-3 py-2.5 bg-alpine-800/80 border border-alpine-700 rounded-lg
                              text-center text-white
                              focus:outline-none focus:ring-2 focus:ring-accent-500/50 ${
                    secours ? 'text-lg font-mono tracking-[0.2em]' : 'text-xl tracking-[0.4em]'
                  }`}
                  placeholder={secours ? 'ABCDE-FGHIJ' : '000000'}
                />
              </div>

              {/* Trente jours : assez pour couvrir un mois de travail sans
                  redemander, assez court pour qu'un portable oublié redevienne
                  protégé avant qu'on ait fini de le chercher. */}
              <label className="flex items-start gap-2 text-sm text-alpine-300 cursor-pointer">
                <input type="checkbox" className="mt-0.5" checked={remember}
                       onChange={e => setRemember(e.target.checked)} />
                <span>
                  {t('cx.seSouvenir')}
                  <span className="block text-xs text-alpine-500">
                    {t('cx.seSouvenirAide')}
                  </span>
                </span>
              </label>

              <button
                type="button"
                onClick={submitCode}
                disabled={loading || !codeComplet}
                className="w-full py-2.5 bg-accent-500 hover:bg-accent-600 text-white font-medium
                           rounded-lg text-sm transition-all duration-150 active:scale-[0.98]
                           disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {loading ? t('at.verificationEnCours') : t('cx.valider')}
              </button>

              {/* Le téléphone perdu ne doit pas enfermer dehors. Le passage au
                  code de secours est un BOUTON, pas une phrase : une consigne
                  qui décrit un mécanisme absent est pire qu'aucune consigne. */}
              <div className="border-t border-alpine-700/60 pt-3">
                <button
                  type="button"
                  onClick={() => { setMode(secours ? 'totp' : 'recovery'); setCode(''); setError('') }}
                  className="text-xs text-accent-400 hover:text-accent-300 flex items-center gap-1.5"
                >
                  {secours
                    ? <><Smartphone size={12} /> {t('connexion.utiliserApp')}</>
                    : <><LifeBuoy size={12} /> {t('connexion.utiliserSecours')}</>}
                </button>
                <p className="text-xs text-alpine-500 mt-1.5">
                  {t(secours ? 'cx.secoursUneFois' : 'cx.secoursNotes')}
                </p>
              </div>

              <button
                type="button"
                onClick={() => {
                  setChallenge(null); setCode(''); setError(''); setMode('totp')
                }}
                className="text-xs text-alpine-400 hover:text-alpine-200 flex items-center gap-1"
              >
                <ArrowLeft size={12} /> {t('connexion.revenir')}
              </button>
            </div>
          ) : (
          <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
            <div>
              <label className="block text-xs font-medium text-alpine-400 mb-1.5 uppercase tracking-wide">
                {t('connexion.email')}
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
              {errors.email && <p className="text-xs text-danger-500 mt-1">{tv(errors.email.message)}</p>}
            </div>

            <div>
              <label className="block text-xs font-medium text-alpine-400 mb-1.5 uppercase tracking-wide">
                {t('connexion.motDePasse')}
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
              {errors.password && <p className="text-xs text-danger-500 mt-1">{tv(errors.password.message)}</p>}
            </div>

            <button
              type="submit"
              disabled={loading}
              className="w-full py-2.5 bg-accent-500 hover:bg-accent-600 text-white font-medium
                         rounded-lg text-sm transition-all duration-150 active:scale-[0.98]
                         disabled:opacity-50 disabled:cursor-not-allowed mt-2"
            >
              {loading ? t('connexion.enCours') : t('connexion.seConnecter')}
            </button>
          </form>
          )}
        </div>

        <p className="text-center text-xs text-alpine-600 mt-6">
          {t('connexion.piedDePage')}
        </p>
      </div>
    </div>
  )
}
