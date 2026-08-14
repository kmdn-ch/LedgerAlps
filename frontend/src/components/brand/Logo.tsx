// L'identité de LedgerAlps.
//
// # Pourquoi elle a besoin d'exister quelque part
//
// La barre latérale affiche le logo et le nom de l'ENTREPRISE de l'utilisateur.
// C'est juste — c'est son espace de travail. Mais dès qu'une entreprise pose son
// logo, plus rien à l'écran ne dit quel logiciel on utilise : ni au support, ni
// à la fiduciaire qui ouvre la session d'un client, ni à celui qui retrouve une
// capture d'écran six mois plus tard.
//
// La marque du produit se pose donc là où elle n'entre pas en concurrence avec
// celle du client : le pied de la barre latérale, l'écran de connexion, et
// l'onglet du navigateur.
//
// # Pourquoi `currentColor`
//
// Le monogramme se pose sur le bleu nuit de la barre latérale ET sur le blanc
// des écrans clairs. Une couleur figée aurait imposé deux fichiers qui
// divergent ; ici les lettres prennent la couleur du texte ambiant, et seul le
// drapeau garde la sienne — le rouge fédéral suisse ne se décline pas.
//
// # Sur la provenance de ce tracé
//
// Il est reconstruit à partir des images de la marque : proportions relevées,
// géométrie réécrite proprement. Si le fichier vectoriel d'origine existe, il
// remplace ce tracé et `frontend/public/logo.svg` sans qu'aucun appelant ne
// bouge — c'est tout l'intérêt d'avoir enfermé la marque dans ce fichier.

const ROUGE_SUISSE = '#D52B1E'

/**
 * Le monogramme seul : « LA » et le drapeau.
 *
 * Les lettres prennent `currentColor`. Posez-le dans un conteneur qui fixe la
 * couleur du texte — `text-white` sur un fond sombre, `text-alpine-900` sinon.
 */
export function LogoMark({ className = '', title }: { className?: string; title?: string }) {
  return (
    <svg
      viewBox="0 0 100 92"
      className={className}
      role={title ? 'img' : 'presentation'}
      aria-label={title}
      aria-hidden={title ? undefined : true}
      focusable="false"
    >
      <g fill="currentColor">
        <rect x="0" y="0" width="14" height="92" rx="3" />
        <rect x="0" y="78" width="57" height="14" rx="3" />
        <path
          d="M26 66 L56 2 L94 90"
          fill="none"
          stroke="currentColor"
          strokeWidth="14"
          strokeLinejoin="round"
        />
        <rect x="30" y="54.5" width="45" height="13" />
      </g>
      <g>
        <rect x="79" y="0" width="21" height="21" fill={ROUGE_SUISSE} />
        <rect x="82.95" y="8.55" width="13.1" height="3.9" fill="#fff" />
        <rect x="87.55" y="3.95" width="3.9" height="13.1" fill="#fff" />
      </g>
    </svg>
  )
}

/**
 * Le drapeau seul, en exposant du mot « LedgerAlps ».
 *
 * Le logotype est du TEXTE, pas une image : il reste net à toutes les tailles,
 * se sélectionne, se lit par un lecteur d'écran, et suit la police de
 * l'interface. Seul le drapeau demande un tracé.
 */
function Drapeau({ className = '' }: { className?: string }) {
  return (
    <svg viewBox="0 0 21 21" className={className} aria-hidden="true" focusable="false">
      <rect x="0" y="0" width="21" height="21" fill={ROUGE_SUISSE} />
      <rect x="3.95" y="8.55" width="13.1" height="3.9" fill="#fff" />
      <rect x="8.55" y="3.95" width="3.9" height="13.1" fill="#fff" />
    </svg>
  )
}

/**
 * Le logotype complet : « LedgerAlps » et son drapeau en exposant.
 *
 * `taille` gouverne la hauteur du texte ; le drapeau suit.
 */
export function Wordmark({
  className = '',
  taille = 'text-base',
}: {
  className?: string
  taille?: string
}) {
  return (
    <span className={`inline-flex items-start font-display font-700 ${taille} ${className}`}>
      <span>LedgerAlps</span>
      {/* En exposant, collé au « s » — c'est la position de la marque. La
          hauteur suit celle du texte (`em`) pour que le rapport tienne à
          toutes les tailles. */}
      <Drapeau className="w-[0.36em] h-[0.36em] ml-[0.06em] mt-[0.1em] flex-shrink-0" />
    </span>
  )
}

/**
 * Marque et logotype côte à côte — l'usage de l'écran de connexion.
 */
export function Logo({ className = '', taille = 'text-xl' }: { className?: string; taille?: string }) {
  return (
    <span className={`inline-flex items-center gap-2.5 ${className}`}>
      <LogoMark className="h-7 w-auto" title="LedgerAlps" />
      <Wordmark taille={taille} />
    </span>
  )
}
