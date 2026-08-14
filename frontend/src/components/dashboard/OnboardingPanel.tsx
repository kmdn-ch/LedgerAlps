// La mise en route, en tête du tableau de bord.
//
// # Ce qu'elle remplace
//
// Une installation neuve s'ouvrait sur quatre compteurs à zéro et un graphique
// vide. Rien n'annonçait que sans adresse structurée le bulletin QR serait
// refusé, ni que sans IBAN le PDF sortirait sans section de paiement. Le
// contrôle de cohérence le disait déjà — dans Paramètres → Maintenance →
// Diagnostic, là où un débutant ne va jamais.
//
// # Ce qu'elle n'est pas
//
// Ce n'est pas un assistant : rien n'est mémorisé, aucune étape n'est
// « validée ». L'état se relit des données à chaque ouverture, si bien qu'un
// IBAN effacé décoche sa case tout seul. Un assistant, lui, aurait retenu
// « fait » et menti à partir de là.
//
// # Pourquoi elle ne s'affiche pas pour tout le monde
//
// Chacune de ses étapes demande PermManage ou l'écriture de documents. Un
// compte en lecture seule ne peut en accomplir aucune : lui présenter une liste
// de choses à faire dont il est écarté ne l'aide pas, cela lui reproche
// simplement son rôle.

import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { Check, ArrowRight, Circle } from 'lucide-react'
import { onboardingApi } from '@/api/client'
import { useCanWrite } from '@/hooks/usePermissions'
import { useT } from '@/i18n/useT'
import type { Cle } from '@/i18n'
import type { Onboarding, OnboardingStep } from '@/types'

// Chaque étape : son libellé, ce qu'elle apporte, et où elle se règle.
//
// La table est indexée par la clé que le serveur envoie. Une étape ajoutée
// là-bas sans sa ligne ici est ignorée plutôt qu'affichée en clé nue — mieux
// vaut une liste incomplète qu'une ligne illisible.
const ÉTAPES: Record<string, { titre: Cle; aide: Cle; vers: string }> = {
  identity: { titre: 'mr.identiteTitre', aide: 'mr.identiteAide', vers: '/settings#identity' },
  uid:      { titre: 'mr.ideTitre',      aide: 'mr.ideAide',      vers: '/settings#identity' },
  vat:      { titre: 'mr.tvaTitre',      aide: 'mr.tvaAide',      vers: '/settings#banking' },
  iban:     { titre: 'mr.ibanTitre',     aide: 'mr.ibanAide',     vers: '/settings#banking' },
  customer: { titre: 'mr.clientTitre',   aide: 'mr.clientAide',   vers: '/contacts' },
  invoice:  { titre: 'mr.factureTitre',  aide: 'mr.factureAide',  vers: '/invoices/new' },
}

// Ce qui bloque, nommé. « Il manque quelque chose » envoie chercher ; « il
// manque la localité » fait agir.
const MANQUANTS: Record<string, Cle> = {
  company_name: 'mr.champRaisonSociale',
  postal_code:  'mr.champNPA',
  city:         'mr.champLocalite',
  country:      'mr.champPays',
  uid_missing:  'mr.ideAbsent',
  uid_invalid:  'mr.ideFormat',
  iban_missing: 'mr.ibanAbsent',
  iban_invalid: 'mr.ibanInvalide',
  vat_undeclared:     'mr.tvaNonDeclare',
  vat_number_missing: 'mr.tvaNumeroManquant',
}

export function OnboardingPanel() {
  const t = useT()
  const peutAgir = useCanWrite()

  const { data } = useQuery<Onboarding>({
    queryKey: ['onboarding'],
    queryFn:  () => onboardingApi.get().then(r => r.data),
    enabled:  peutAgir,
    // Relue à chaque retour sur le tableau de bord, sans passer par le cache
    // de trente secondes commun aux autres requêtes.
    //
    // Cinq écrans différents peuvent cocher une de ces étapes : les réglages,
    // les contacts, la création de facture, l'import d'une facture
    // fournisseur, et demain un autre. Compter sur ce que chacun pense à
    // invalider, c'est se garantir que le sixième oubliera — et quelqu'un qui
    // vient de saisir sa localité lirait « il manque la localité ». Le coût
    // est un GET de cinq compteurs, et il cesse dès que la liste disparaît.
    staleTime:      0,
    refetchOnMount: 'always',
  })

  // Terminée, la liste disparaît — c'est la seule façon honnête de la faire
  // partir. Un bouton « masquer » laisserait une installation incomplète avec
  // un écran qui ne le dit plus.
  if (!peutAgir || !data || data.complete) return null

  const étapes = data.steps.filter(e => ÉTAPES[e.key])

  return (
    <div className="card mb-6 border-accent-200 bg-accent-50/40">
      <div className="card-header">
        <h2 className="text-sm font-semibold text-alpine-800">{t('mr.titre')}</h2>
        <span className="text-xs text-alpine-500 tabular-nums">
          {t('mr.progression', { faites: data.done_count, total: data.total })}
        </span>
      </div>

      <div className="card-body pt-3">
        <p className="text-xs text-alpine-600 mb-3">{t('mr.intro')}</p>
        <ul className="space-y-1">
          {étapes.map(e => <Étape key={e.key} etape={e} />)}
        </ul>
      </div>
    </div>
  )
}

function Étape({ etape }: { etape: OnboardingStep }) {
  const t = useT()
  const def = ÉTAPES[etape.key]

  // Faite, l'étape reste visible et cochée : la progression est ce qui rend
  // une liste supportable, et une ligne qui s'efface donne l'impression que le
  // travail n'a pas compté.
  if (etape.done) {
    return (
      <li className="flex items-center gap-2 py-1.5 text-sm text-alpine-500">
        <Check size={15} className="flex-shrink-0 text-success-700" />
        <span className="line-through decoration-alpine-300">{t(def.titre)}</span>
      </li>
    )
  }

  const bloquants = (etape.missing ?? [])
    .map(m => MANQUANTS[m])
    .filter(Boolean)
    .map(cle => t(cle as Cle))

  return (
    <li>
      <Link
        to={def.vers}
        className="group flex items-start gap-2 py-1.5 rounded-md
                   hover:bg-white/70 transition-colors"
      >
        <Circle size={15} className="flex-shrink-0 mt-0.5 text-alpine-300" />
        <span className="flex-1 min-w-0">
          <span className="text-sm font-medium text-alpine-800">{t(def.titre)}</span>
          <span className="block text-xs text-alpine-600">{t(def.aide)}</span>
          {bloquants.length > 0 && (
            <span className="block text-xs text-danger-700 mt-0.5">
              {t('mr.ilManque', { champs: bloquants.join(', ') })}
            </span>
          )}
        </span>
        <ArrowRight
          size={14}
          className="flex-shrink-0 mt-0.5 text-alpine-300 group-hover:text-accent-600
                     transition-colors"
        />
      </Link>
    </li>
  )
}
