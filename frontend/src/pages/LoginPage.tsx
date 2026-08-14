// LedgerAlps — Page de connexion
//
// # Ce qui vient du gabarit, et ce qui ne pouvait pas en venir
//
// La MISE EN PAGE est celle du gabarit validé : deux panneaux, l'un qui porte
// la marque et ce que le produit garantit, l'autre qui demande les
// identifiants. Les couleurs sont ses couleurs exactes (`brand.*` dans la
// configuration Tailwind), et non une approximation avec la palette du reste
// de l'application.
//
// Trois choses du gabarit n'ont PAS été reprises, et chacune pour une raison :
//
//   - Les CDN — Tailwind, Font Awesome, Google Fonts. La politique de sécurité
//     du serveur les bloque, et surtout : chaque appel transmettrait l'adresse
//     IP de l'utilisateur à un tiers, ce qui contredit « vos données restent
//     sur votre machine ». Les icônes viennent de lucide-react, déjà embarqué.
//
//   - Le logo redessiné en HTML. Le gabarit le reconstruit avec du texte et un
//     carré rouge ; le fichier officiel existe et c'est lui qui s'affiche.
//
//   - « Oublié ? » et « Rester connecté ». LedgerAlps n'envoie aucun courriel :
//     un lien de réinitialisation ne mènerait nulle part. Et il n'existe pas de
//     session longue au niveau du mot de passe — la seule mémoire réelle est
//     celle du second facteur, qui a son propre réglage à l'étape suivante. Un
//     bouton qui ment coûte plus cher qu'un bouton absent.
//
// # Ce qui a été ajouté
//
//   - Le choix de la langue, en pied de page. Il vivait derrière la connexion,
//     ce qui obligeait à lire le français pour trouver comment ne plus le lire.
//
//   - Un conseil de sécurité, tiré au sort et frappé à la machine. L'écran de
//     connexion est le seul moment de la journée où l'utilisateur n'a rien
//     d'autre à faire que regarder ; la même consigne dans un manuel n'est
//     jamais lue.
//
//   - Le pied du formulaire porte la VERSION INSTALLÉE, lue au serveur. C'est
//     la première chose qu'on demande dans un ticket de support, et la
//     dernière qu'on trouve.

import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useNavigate, useSearchParams } from 'react-router-dom'
import {
  Eye, EyeOff, Smartphone, ArrowLeft, ArrowRight, LifeBuoy,
  Mail, Lock, Server, Scale, Loader2,
} from 'lucide-react'
import { LedgerAlpsLogo } from '@/components/brand/Logo'
import { SelecteurLangue } from '@/components/brand/SelecteurLangue'
import { ConseilSecurite } from '@/components/brand/ConseilSecurite'
import { authApi, healthApi } from '@/api/client'
import { useAuthStore } from '@/store/auth'
import { useT, useTv } from '@/i18n/useT'

const schema = z.object({
  email:    z.string().email('val.emailInvalide'),
  password: z.string().min(8, 'cx.motDePasseCourt'),
})
type FormData = z.infer<typeof schema>

export function LoginPage() {
  const t = useT()

  /**
   * Des secondes en une durée qu'on lit.
   *
   * « Réessayez dans 3600 secondes » est exact et inutilisable. Les paliers du
   * serveur — 30 s, 1 min, 5 min, 15 min, 1 h — tombent tous juste, et
   * l'arrondi supérieur évite d'annoncer « 0 minute » sur les derniers
   * instants d'un verrou.
   */
  const enClair = (secondes: number): string => {
    if (secondes >= 3600) return t('duree.uneHeure')
    if (secondes >= 120)  return t('duree.minutes', { n: Math.ceil(secondes / 60) })
    if (secondes >= 60)   return t('duree.uneMinute')
    return t('duree.secondes', { n: Math.max(1, Math.ceil(secondes)) })
  }
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
  // impossible à taper : la moitié se perdait au sixième caractère.
  const [mode, setMode] = useState<'totp' | 'recovery'>('totp')
  // Décoché par défaut, toujours. Une protection qu'on lève sans le savoir
  // n'en est plus une : la dispense doit être un geste conscient.
  const [remember, setRemember] = useState(false)
  const secours = mode === 'recovery'

  // La version INSTALLÉE, lue au serveur. Elle n'est pas compilée dans la page :
  // le paquet du navigateur et le binaire se mettent à jour ensemble, mais c'est
  // le binaire qui fait foi, et c'est lui qu'on veut lire dans un ticket de
  // support.
  const [version, setVersion] = useState('')
  useEffect(() => {
    let vivant = true
    healthApi.get()
      .then(r => { if (vivant) setVersion(String(r.data?.version ?? '')) })
      .catch(() => { /* le pied de page reste alors vide, plutôt que de mentir */ })
    return () => { vivant = false }
  }, [])

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
        const secs = (e as { response?: { data?: { retry_after?: number } } })
          .response?.data?.retry_after ?? 0
        setError(t('cx.verrouille', { duree: enClair(secs) }))
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
    } catch (e) {
      // Un compte verrouillé ne doit PAS lire « identifiants incorrects » : il
      // réessaierait, prolongeant l'attente à chaque série. Le serveur donne le
      // délai qui reste ; on le rend lisible.
      const rep = (e as { response?: { status?: number; data?: { retry_after?: number } } }).response
      if (rep?.status === 429) {
        setError(t('cx.verrouille', { duree: enClair(rep.data?.retry_after ?? 0) })
                 + ' ' + t('cx.verrouilleAide'))
      } else {
        setError(t('connexion.identifiantsIncorrects'))
      }
    } finally {
      setLoading(false)
    }
  }

  const champ = 'w-full py-3 bg-brand-navyInput border border-brand-navyBorder/80 rounded-xl ' +
                'text-slate-100 text-sm placeholder:text-slate-500 focus:outline-none ' +
                'focus:ring-2 focus:ring-brand-orange/60 focus:border-brand-orange transition-all'

  return (
    <div className="min-h-screen flex flex-col bg-brand-navyBg text-slate-100 antialiased">

      <main className="flex-grow flex items-center justify-center p-4 sm:p-6 lg:p-10">
        <div className="w-full max-w-5xl space-y-5">

          {idle && (
            <div className="rounded-xl border border-warning-500/40 bg-warning-500/10
                            px-4 py-3 text-sm">
              <p className="font-medium text-warning-500">{t('connexion.sessionFermee')}</p>
              <p className="mt-0.5 text-slate-400 text-xs">{t('cx.deconnexionAuto')}</p>
            </div>
          )}

          <div className="bg-brand-navyCard border border-brand-navyBorder rounded-3xl
                          overflow-hidden shadow-2xl grid grid-cols-1 md:grid-cols-12
                          min-h-[580px]">

            {/* ── Panneau gauche : la marque et ce qui est garanti ─────────── */}
            <div className="md:col-span-5 bg-gradient-to-br from-slate-950 via-brand-navyBg
                            to-slate-900 p-8 sm:p-10 flex flex-col justify-between
                            border-b md:border-b-0 md:border-r border-slate-800/80
                            relative overflow-hidden">

              <div className="space-y-8 z-10 relative">
                {/* Le fichier officiel, sur sa plaque claire — la marque est en
                    bleu nuit et ne se poserait pas sur ce fond. */}
                <div className="bg-white rounded-2xl px-6 py-3.5 shadow-md inline-flex
                                items-center justify-center">
                  <LedgerAlpsLogo className="h-7 w-auto" />
                </div>

                <div className="space-y-3">
                  <h1 className="text-2xl sm:text-3xl font-display font-700 text-white
                                 tracking-tight leading-snug">
                    {t('cx.accrocheTitre')}
                  </h1>
                  <p className="text-slate-400 text-xs sm:text-sm leading-relaxed">
                    {t('cx.accrocheSousTitre')}
                  </p>
                </div>

                {/* Le conseil du jour. Placé sous la marque, dans l'espace qui
                    restait vide : c'est le seul moment où l'utilisateur n'a
                    rien d'autre à faire que regarder. */}
                <ConseilSecurite className="pt-2 border-t border-slate-800/60" />
              </div>

              <div className="space-y-3.5 pt-8 z-10 relative border-t border-slate-800/80
                              mt-8 md:mt-0">
                <Garantie
                  icone={<Server size={13} />}
                  couleur="bg-emerald-500/10 text-emerald-400 border-emerald-500/20"
                  titre={t('cx.garantieLocalTitre')}
                  detail={t('cx.garantieLocalAide')}
                />
                <Garantie
                  icone={<Scale size={13} />}
                  couleur="bg-brand-orange/10 text-brand-orange border-brand-orange/20"
                  titre={t('cx.garantieLegaleTitre')}
                  detail={t('cx.garantieLegaleAide')}
                />
              </div>

              <div className="absolute -bottom-24 -right-24 w-72 h-72 bg-brand-orange/5
                              rounded-full blur-3xl pointer-events-none" />
            </div>

            {/* ── Panneau droit : les identifiants ─────────────────────────── */}
            <div className="md:col-span-7 p-8 sm:p-12 flex flex-col justify-between">
              <div className="max-w-sm w-full mx-auto space-y-7 my-auto">

                <div className="space-y-1.5">
                  <h2 className="text-2xl font-display font-700 text-white tracking-tight">
                    {challenge ? t('connexion.verification') : t('connexion.titre')}
                  </h2>
                  <p className="text-xs text-slate-400">
                    {!challenge
                      ? t('connexion.sousTitre')
                      : secours
                        ? t('connexion.saisirCodeSecours')
                        : t('connexion.saisirCodeApp')}
                  </p>
                </div>

                {error && (
                  <div className="p-3 rounded-xl text-xs font-medium bg-danger-500/10
                                  text-danger-500 border border-danger-500/20">
                    {error}
                  </div>
                )}

                {/* Deuxième étape : le mot de passe est accepté, la session
                    n'existe pas encore. Rien de ce qui s'affiche ici n'ouvre
                    quoi que ce soit tant que le code n'est pas validé. */}
                {challenge ? (
                  <div className="space-y-5">
                    <div className="flex items-start gap-2.5 text-xs text-slate-300">
                      {secours
                        ? <LifeBuoy size={15} className="text-brand-orange mt-0.5 shrink-0" />
                        : <Smartphone size={15} className="text-brand-orange mt-0.5 shrink-0" />}
                      <span>
                        {secours ? t('connexion.aideCodeSecours') : t('connexion.aideCodeApp')}
                        <strong className="text-white">{challenge.email}</strong>.
                      </span>
                    </div>

                    <div className="space-y-2">
                      <label htmlFor="otp"
                             className="block text-xs font-semibold tracking-wider
                                        text-slate-300 uppercase">
                        {t(secours ? 'cx.champCodeSecours' : 'cx.champCodeSixChiffres')}
                      </label>
                      {/* Le champ change vraiment de nature selon le mode :
                          longueur, clavier, casse, espacement. */}
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
                        className={`${champ} px-4 text-center ${
                          secours ? 'text-lg font-mono tracking-[0.2em]' : 'text-xl tracking-[0.4em]'
                        }`}
                        placeholder={secours ? 'ABCDE-FGHIJ' : '000000'}
                      />
                    </div>

                    {/* Trente jours : assez pour couvrir un mois de travail sans
                        redemander, assez court pour qu'un portable oublié
                        redevienne protégé avant qu'on ait fini de le chercher. */}
                    <label className="flex items-start gap-2.5 text-xs text-slate-400 cursor-pointer group">
                      <input type="checkbox" checked={remember}
                             onChange={e => setRemember(e.target.checked)}
                             className="mt-0.5 w-4 h-4 rounded border-slate-700 bg-brand-navyInput
                                        accent-brand-orange cursor-pointer" />
                      <span>
                        <span className="group-hover:text-slate-300">{t('cx.seSouvenir')}</span>
                        <span className="block text-[11px] text-slate-500">{t('cx.seSouvenirAide')}</span>
                      </span>
                    </label>

                    <button type="button" onClick={submitCode} disabled={loading || !codeComplet}
                            className="w-full py-3.5 px-4 bg-brand-orange hover:bg-brand-orangeHover
                                       active:scale-[0.99] text-white font-medium rounded-xl text-sm
                                       shadow-md transition-all flex items-center justify-center gap-2
                                       disabled:opacity-50 disabled:cursor-not-allowed">
                      {loading
                        ? <><Loader2 size={14} className="animate-spin" /> {t('at.verificationEnCours')}</>
                        : <>{t('cx.valider')} <ArrowRight size={13} /></>}
                    </button>

                    {/* Le téléphone perdu ne doit pas enfermer dehors. Le passage
                        au code de secours est un BOUTON, pas une phrase. */}
                    <div className="border-t border-slate-800/80 pt-4 space-y-3">
                      <button
                        type="button"
                        onClick={() => { setMode(secours ? 'totp' : 'recovery'); setCode(''); setError('') }}
                        className="text-xs text-brand-orange hover:underline font-medium
                                   flex items-center gap-1.5"
                      >
                        {secours
                          ? <><Smartphone size={12} /> {t('connexion.utiliserApp')}</>
                          : <><LifeBuoy size={12} /> {t('connexion.utiliserSecours')}</>}
                      </button>
                      <p className="text-[11px] text-slate-500">
                        {t(secours ? 'cx.secoursUneFois' : 'cx.secoursNotes')}
                      </p>
                      <button
                        type="button"
                        onClick={() => { setChallenge(null); setCode(''); setError(''); setMode('totp') }}
                        className="text-xs text-slate-400 hover:text-slate-200 flex items-center gap-1"
                      >
                        <ArrowLeft size={12} /> {t('connexion.revenir')}
                      </button>
                    </div>
                  </div>
                ) : (
                  <form onSubmit={handleSubmit(onSubmit)} className="space-y-5">
                    <div className="space-y-2">
                      <label htmlFor="email"
                             className="block text-xs font-semibold tracking-wider
                                        text-slate-300 uppercase">
                        {t('connexion.email')}
                      </label>
                      <div className="relative">
                        <span className="absolute inset-y-0 left-0 pl-3.5 flex items-center
                                         pointer-events-none text-slate-500">
                          <Mail size={15} />
                        </span>
                        <input
                          id="email"
                          type="email"
                          autoComplete="email"
                          className={`${champ} pl-10 pr-4 ${errors.email ? 'border-danger-500' : ''}`}
                          placeholder="vous@exemple.ch"
                          {...register('email')}
                        />
                      </div>
                      {errors.email && (
                        <p className="text-xs text-danger-500">{tv(errors.email.message)}</p>
                      )}
                    </div>

                    <div className="space-y-2">
                      <label htmlFor="password"
                             className="block text-xs font-semibold tracking-wider
                                        text-slate-300 uppercase">
                        {t('connexion.motDePasse')}
                      </label>
                      <div className="relative">
                        <span className="absolute inset-y-0 left-0 pl-3.5 flex items-center
                                         pointer-events-none text-slate-500">
                          <Lock size={15} />
                        </span>
                        <input
                          id="password"
                          type={showPw ? 'text' : 'password'}
                          autoComplete="current-password"
                          className={`${champ} pl-10 pr-11 ${errors.password ? 'border-danger-500' : ''}`}
                          placeholder="••••••••••••"
                          {...register('password')}
                        />
                        <button
                          type="button"
                          onClick={() => setShowPw(!showPw)}
                          title={t(showPw ? 'cx.masquerMotDePasse' : 'cx.afficherMotDePasse')}
                          className="absolute inset-y-0 right-0 pr-3.5 flex items-center
                                     text-slate-400 hover:text-slate-200 focus:outline-none"
                        >
                          {showPw ? <EyeOff size={15} /> : <Eye size={15} />}
                        </button>
                      </div>
                      {errors.password && (
                        <p className="text-xs text-danger-500">{tv(errors.password.message)}</p>
                      )}
                    </div>

                    <button
                      type="submit"
                      disabled={loading}
                      className="w-full py-3.5 px-4 bg-brand-orange hover:bg-brand-orangeHover
                                 active:scale-[0.99] text-white font-medium rounded-xl text-sm
                                 shadow-md transition-all flex items-center justify-center gap-2 mt-2
                                 disabled:opacity-50 disabled:cursor-not-allowed"
                    >
                      {loading
                        ? <><Loader2 size={14} className="animate-spin" /> {t('connexion.enCours')}</>
                        : <>{t('connexion.seConnecter')} <ArrowRight size={13} /></>}
                    </button>
                  </form>
                )}
              </div>

              <div className="text-center text-xs text-slate-500 mt-8 tracking-wide">
                {version && `LedgerAlps Version : ${version}`}
              </div>
            </div>
          </div>
        </div>
      </main>

      {/* ── Pied : la langue ───────────────────────────────────────────────── */}
      <footer className="border-t border-slate-800/80 py-4 px-6">
        <div className="max-w-5xl mx-auto flex items-center justify-center">
          <SelecteurLangue />
        </div>
      </footer>
    </div>
  )
}

/** Une garantie du panneau gauche : une pastille, un titre, une précision. */
function Garantie({ icone, couleur, titre, detail }: {
  icone: React.ReactNode; couleur: string; titre: string; detail: string
}) {
  return (
    <div className="flex items-center gap-3 text-xs text-slate-300">
      <div className={`w-7 h-7 rounded-lg flex items-center justify-center shrink-0 border ${couleur}`}>
        {icone}
      </div>
      <div>
        <span className="font-medium block text-slate-200">{titre}</span>
        <span className="text-[11px] text-slate-400">{detail}</span>
      </div>
    </div>
  )
}
