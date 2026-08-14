// LedgerAlps — Sidebar de navigation

import { NavLink } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import {
  LayoutDashboard, FileText, Users, BookOpen,
  BarChart3, Settings, LogOut, ArrowLeftRight, ShoppingCart,
} from 'lucide-react'
import { cn } from '@/utils'
import { useAuthStore } from '@/store/auth'
import { settingsApi, healthApi, authApi } from '@/api/client'
import { AccountBanner } from './AccountBanner'
import { useT } from '@/i18n/useT'
import { LedgerAlpsIcon, LedgerAlpsPlaque } from '@/components/brand/Logo'

const NAV = [
  { to: '/',          icon: LayoutDashboard, cle: 'nav.tableauDeBord' },
  // Un seul libellé : les deux vivent dans le même écran, qui bascule de l'un
  // à l'autre. Deux entrées de menu pour deux vues du même objet donnaient
  // l'impression de deux registres séparés, qu'ils ne sont pas — une offre
  // acceptée devient une facture, et les deux se citent.
  { to: '/invoices',  icon: FileText,        cle: 'nav.facturation'   },
  // Les achats vivent à côté de la facturation : ce sont les deux sens du
  // même flux, et payer un fournisseur commence par saisir sa facture.
  { to: '/purchases', icon: ShoppingCart,    cle: 'nav.achats'        },
  { to: '/contacts',  icon: Users,           cle: 'nav.contacts'      },
  { to: '/journal',   icon: ArrowLeftRight,  cle: 'nav.journal'       },
  { to: '/accounts',  icon: BookOpen,        cle: 'nav.planComptable' },
  { to: '/reports',   icon: BarChart3,       cle: 'nav.rapports'      },
] as const

export function Sidebar() {
  const t = useT()
  const { user, logout } = useAuthStore()

  // Se déconnecter doit révoquer la session côté serveur, pas seulement effacer
  // l'état local : sans cet appel, le jeton de rafraîchissement restait valide
  // trente jours et le cookie HttpOnly ne serait jamais effacé — le code ne peut
  // pas le supprimer lui-même, c'est précisément le principe.
  async function handleLogout() {
    try {
      await authApi.logout()
    } catch {
      // Serveur injoignable ou session déjà expirée : on déconnecte quand même
      // localement, car refuser de déconnecter serait le pire des comportements.
    } finally {
      logout()
    }
  }

  const { data: company } = useQuery({
    queryKey: ['company-settings'],
    queryFn:  () => settingsApi.getCompany().then(r => r.data),
    staleTime: 5 * 60 * 1000,
  })

  const { data: health } = useQuery({
    queryKey: ['health'],
    queryFn:  () => healthApi.get().then(r => r.data),
    staleTime: Infinity,
  })

  const companyName = company?.company_name || 'LedgerAlps'
  const logoData    = company?.logo_data ?? null

  return (
    <aside className="fixed left-0 top-0 h-screen w-[240px] bg-alpine-900 text-white
                      flex flex-col z-30 select-none">
      {/* Logo / Brand */}
      <div className="flex items-center gap-2.5 px-5 py-5 border-b border-alpine-700/50">
        {logoData ? (
          <div className="w-8 h-8 rounded-lg overflow-hidden bg-white flex items-center justify-center flex-shrink-0">
            <img
              src={logoData}
              alt="Logo"
              className="w-full h-full object-contain"
            />
          </div>
        ) : (
          <div className="w-8 h-8 rounded-lg bg-white flex items-center justify-center
                          flex-shrink-0 p-1">
            <LedgerAlpsIcon className="w-full h-full object-contain" />
          </div>
        )}
        <div className="min-w-0">
          <div className="font-display font-700 text-sm leading-none truncate">{companyName}</div>
        </div>
      </div>

      {/* Nav */}
      <nav className="flex-1 px-3 py-4 space-y-0.5 overflow-y-auto">
        {NAV.map(({ to, icon: Icon, cle }) => (
          <NavLink
            key={to}
            to={to}
            end={to === '/'}
            className={({ isActive }) => cn(
              'flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm transition-all duration-150',
              isActive
                ? 'bg-accent-500 text-white font-medium shadow-lg shadow-accent-500/25'
                : 'text-alpine-300 hover:bg-alpine-800 hover:text-white'
            )}
          >
            <Icon size={16} className="flex-shrink-0" />
            <span>{t(cle)}</span>
          </NavLink>
        ))}
      </nav>

      {/* Avec quel compte travaille-t-on ? Un administrateur qui se croit en
          lecture seule modifie sans s'en rendre compte, et un compte
          administrateur laissé ouvert sur un poste partagé est la porte que
          personne ne pense à fermer. */}
      <AccountBanner />

      {/* Footer utilisateur */}
      <div className="border-t border-alpine-700/50 px-3 py-3 space-y-0.5">
        <NavLink
          to="/settings"
          className="flex items-center gap-3 px-3 py-2 rounded-lg text-sm
                     text-alpine-300 hover:bg-alpine-800 hover:text-white transition-all"
        >
          <Settings size={16} />
          <span>{t('nav.parametres')}</span>
        </NavLink>

        <button
          onClick={handleLogout}
          className="w-full flex items-center gap-3 px-3 py-2 rounded-lg text-sm
                     text-alpine-400 hover:bg-danger-500/10 hover:text-danger-500 transition-all"
        >
          <LogOut size={16} />
          <span>{t('nav.deconnexion')}</span>
        </button>

        {user && (
          <div className="px-3 py-2 mt-1 border-t border-alpine-700/50">
            <div className="text-xs font-medium text-white truncate">{user.name}</div>
            <div className="text-[10px] text-alpine-400 truncate">{user.email}</div>
            {/* La marque du PRODUIT, ici et pas en haut.
                En haut vit l'entreprise de l'utilisateur — son logo, son nom —
                et lui disputer cette place serait malpoli. En bas, elle ne
                gêne personne et répond à la seule question qu'on se pose
                devant une capture d'écran : quel logiciel est-ce ? */}
            <div className="flex items-baseline justify-between gap-2 mt-2 pt-2
                            border-t border-alpine-800">
              <LedgerAlpsPlaque hauteur="h-3" />
              {health?.version && (
                <span className="text-[9px] text-alpine-600 tabular-nums flex-shrink-0">
                  {health.version}
                </span>
              )}
            </div>
          </div>
        )}
      </div>
    </aside>
  )
}
