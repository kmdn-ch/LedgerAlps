// Choisir la langue de l'interface.
//
// # Visible pour TOUS les rôles
//
// La langue n'est pas un réglage d'administration : c'est une préférence
// d'affichage personnelle. Une fiduciaire tessinoise à qui l'on ouvre les
// livres en lecture seule doit pouvoir lire en italien sans demander à
// personne. Ce panneau vit donc dans « Mon compte », comme le second facteur.
//
// # Pas besoin de se reconnecter
//
// Le changement est immédiat : le catalogue est embarqué, rien n'est à
// recharger. Faire redémarrer une session pour changer une langue laisserait
// croire qu'il se passe quelque chose de plus lourd, et perdrait la saisie en
// cours.
//
// # L'avertissement n'est pas de la modestie
//
// La traduction est en cours. Le dire là où l'on choisit la langue évite qu'un
// utilisateur croie à une panne en voyant les écrans rester en français, et
// évite de lui faire perdre du temps à chercher un réglage qui n'existe pas.
// Il disparaîtra quand la couverture sera complète — le CHANGELOG le dira.

import { Languages, Info } from 'lucide-react'
import { SectionTitle } from '@/components/ui'
import { LANGUES, type Langue } from '@/i18n'
import { useT, useLangueStore } from '@/i18n/useT'

export function LanguagePanel() {
  const t = useT()
  const langue = useLangueStore(s => s.langue)
  const definir = useLangueStore(s => s.definir)

  return (
    <div className="mt-6">
      <SectionTitle>{t('langue.titre')}</SectionTitle>
      <p className="text-sm text-alpine-600 mb-3">{t('langue.description')}</p>

      <div className="flex flex-wrap gap-2">
        {LANGUES.map(l => (
          <button
            key={l.code}
            type="button"
            onClick={() => definir(l.code as Langue)}
            aria-pressed={langue === l.code}
            className={`btn-sm flex items-center gap-1.5 ${
              langue === l.code ? 'btn-primary' : 'btn-secondary'
            }`}
          >
            <Languages size={13} />
            <span className="font-mono text-[11px]">{l.drapeau}</span>
            {l.nom}
          </button>
        ))}
      </div>

      {/* Dire où en est la traduction, plutôt que de laisser découvrir. */}
      <div className="mt-3 flex items-start gap-2 rounded-md border border-alpine-200
                      bg-alpine-50 px-3 py-2">
        <Info size={14} className="text-alpine-500 flex-shrink-0 mt-0.5" />
        <p className="text-xs text-alpine-600">
          <strong>{t('langue.encoursTitre')}</strong> — {t('langue.encoursDetail')}
        </p>
      </div>
    </div>
  )
}
