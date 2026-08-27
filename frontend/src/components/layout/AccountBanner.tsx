// Avec quel compte travaillez-vous ?
//
// La question paraît triviale ; elle ne l'est pas. Un administrateur qui croit
// être en lecture seule modifie sans s'en rendre compte. Une fiduciaire qui
// croit pouvoir corriger perd son travail sur un refus. Et un compte
// administrateur laissé ouvert sur un poste partagé est la porte que personne
// ne pense à fermer.
//
// Le bandeau est en bas du menu, visible en permanence, et il dit ce que le
// rôle IMPLIQUE plutôt que son seul nom : « Administrateur » n'apprend rien à
// qui ne connaît pas le modèle de droits.
//
// Il ne protège de rien par lui-même. Les droits se vérifient dans la base à
// chaque requête, et modifier cette valeur dans le navigateur ne donne aucun
// accès. C'est un rappel, pas une barrière.

import { ShieldAlert, Eye, Calculator } from 'lucide-react'
import { useAuthStore } from '@/store/auth'
import { useRole } from '@/hooks/usePermissions'
import { useT } from '@/i18n/useT'

export function AccountBanner() {
  const t = useT()
  const role = useRole()
  const name = useAuthStore(s => s.user?.name)

  if (!role) return null

  if (role === 'admin') {
    return (
      <div className="mx-3 mb-3 rounded-md border border-warning-500 bg-warning-100 px-3 py-2">
        <p className="flex items-start gap-1.5 text-[11px] font-semibold text-warning-700 leading-snug">
          <ShieldAlert size={13} className="mt-0.5 flex-shrink-0" />
          <span>
            {t('banniere.adminTitre')}
            <span className="block font-normal text-alpine-700 mt-0.5">
              {t('banniere.adminDetail')}
            </span>
          </span>
        </p>
      </div>
    )
  }

  if (role === 'viewer') {
    return (
      <div className="mx-3 mb-3 rounded-md border border-alpine-600 bg-alpine-800 px-3 py-2">
        <p className="flex items-start gap-1.5 text-[11px] font-semibold text-alpine-100 leading-snug">
          <Eye size={13} className="mt-0.5 flex-shrink-0" />
          <span>
            {t('role.lectureSeuleTitre')}
            <span className="block font-normal text-alpine-300 mt-0.5">
              {t('role.lectureSeuleDetail')}
            </span>
          </span>
        </p>
      </div>
    )
  }

  return (
    <div className="mx-3 mb-3 rounded-md border border-alpine-700 bg-alpine-800/60 px-3 py-2">
      <p className="flex items-start gap-1.5 text-[11px] font-semibold text-alpine-100 leading-snug">
        <Calculator size={13} className="mt-0.5 flex-shrink-0" />
        <span>
          {t('banniere.comptableTitre')}{name ? ` — ${name}` : ''}
          <span className="block font-normal text-alpine-300 mt-0.5">
            {t('banniere.comptableDetail')}
          </span>
        </span>
      </p>
    </div>
  )
}
