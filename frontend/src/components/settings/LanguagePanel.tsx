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
// # L'avertissement a été retiré, et c'est un fait, pas une opinion
//
// Ce panneau a porté un « Traduction en cours » tant que les écrans restaient
// en français. La couverture est désormais complète, vérifiée par
// `internal/frontend/i18n_test.go` : aucune valeur d'un catalogue ne vaut plus
// le français, et aucune clé ne manque. Un avertissement qu'on laisse après
// coup est pire que pas d'avertissement — il apprend à ne plus les lire.

import { Languages } from 'lucide-react'
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
    </div>
  )
}
