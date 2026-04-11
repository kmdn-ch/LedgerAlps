// LedgerAlps — Sidebar de navigation

import { NavLink } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import {
  LayoutDashboard, FileText, Users, BookOpen,
  BarChart3, Settings, LogOut, Mountain,
  Receipt, ArrowLeftRight,
} from 'lucide-react'
import { cn } from '@/utils'
import { useAuthStore } from '@/store/auth'
import { settingsApi } from '@/api/client'

const NAV = [
  { to: '/',          icon: LayoutDashboard, label: 'Tableau de bord' },
  { to: '/invoices',  icon: FileText,        label: 'Factures'        },
  { to: '/quotes',    icon: Receipt,         label: 'Offres de prix'  },
  { to: '/contacts',  icon: Users,           label: 'Contacts'        },
  { to: '/journal',   icon: ArrowLeftRight,  label: 'Journal'         },
  { to: '/accounts',  icon: BookOpen,        label: 'Plan comptable'  },
  { to: '/reports',   icon: BarChart3,       label: 'Rapports'        },
]

export function Sidebar() {
  const { user, logout } = useAuthStore()

  const { data: company } = useQuery({
    queryKey: ['company-settings'],
    queryFn:  () => settingsApi.getCompany().then(r => r.data),
    staleTime: 5 * 60 * 1000,
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
          <div className="w-8 h-8 rounded-lg bg-accent-500 flex items-center justify-center
                          shadow-lg shadow-accent-500/30 flex-shrink-0">
            <Mountain className="w-4.5 h-4.5 text-white" size={18} />
          </div>
        )}
        <div className="min-w-0">
          <div className="font-display font-700 text-sm leading-none truncate">{companyName}</div>
          <div className="text-[10px] text-alpine-400 mt-0.5 leading-none">Comptabilité CH</div>
        </div>
      </div>

      {/* Nav */}
      <nav className="flex-1 px-3 py-4 space-y-0.5 overflow-y-auto">
        {NAV.map(({ to, icon: Icon, label }) => (
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
            <span>{label}</span>
          </NavLink>
        ))}
      </nav>

      {/* Footer utilisateur */}
      <div className="border-t border-alpine-700/50 px-3 py-3 space-y-0.5">
        <NavLink
          to="/settings"
          className="flex items-center gap-3 px-3 py-2 rounded-lg text-sm
                     text-alpine-300 hover:bg-alpine-800 hover:text-white transition-all"
        >
          <Settings size={16} />
          <span>Paramètres</span>
        </NavLink>

        <button
          onClick={logout}
          className="w-full flex items-center gap-3 px-3 py-2 rounded-lg text-sm
                     text-alpine-400 hover:bg-danger-500/10 hover:text-danger-400 transition-all"
        >
          <LogOut size={16} />
          <span>Déconnexion</span>
        </button>

        {user && (
          <div className="px-3 py-2 mt-1 border-t border-alpine-700/50">
            <div className="text-xs font-medium text-white truncate">{user.name}</div>
            <div className="text-[10px] text-alpine-400 truncate">{user.email}</div>
          </div>
        )}
      </div>
    </aside>
  )
}
