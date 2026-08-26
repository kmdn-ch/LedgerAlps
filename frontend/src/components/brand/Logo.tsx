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
// celle du client : le haut à DROITE de l'espace de travail, le pied de la barre
// latérale, l'écran de connexion, et l'onglet du navigateur.
//
// # D'où viennent ces images, et pourquoi on n'y touche pas
//
// Elles vivent dans `infrastructure/brand/`, et `frontend/public/` en porte la
// copie que le binaire embarque. Ce composant ne dessine rien lui-même : il pose
// les fichiers.
//
// Le LOGOTYPE est vectorisé depuis `LOGO.png`, la version fournie. Une
// reconstruction à la police avait précédé ces fichiers ; elle perdait
// l'espacement et le détail des raccords, et c'est pourquoi on décalque au lieu
// de retaper. Seul le badge suisse est CONSTRUIT plutôt que décalqué : un
// drapeau suisse a une géométrie fixée par l'ordonnance (RS 232.21), et la
// décalquer revenait à en recopier les approximations — l'ancien fichier portait
// un rouge #C42527 au lieu du #DA291C qui fait foi.
//
// L'ICÔNE, elle, est toujours le monogramme « LA » d'origine, intact.
//
// # Ce que cela impose à l'écran
//
// La marque est en bleu nuit, sur fond transparent. Elle ne se pose donc pas
// telle quelle sur un fond sombre : là où le support est foncé — la barre
// latérale, l'écran de connexion — elle est présentée sur une plaque claire.
// C'est un choix de MISE EN PAGE, pas une retouche : recolorer le logo pour
// l'accommoder reviendrait à en faire un autre.

/** Le monogramme « LA » et son drapeau. Rapport ≈ 1,12 : 1. */
export function LedgerAlpsIcon({ className = '' }: { className?: string }) {
  return <img src="/ledgeralps-icon.svg" alt="" aria-hidden="true" className={className} />
}

/** Le logotype « LedgerAlps » et son badge suisse. Rapport 580 : 152 ≈ 3,82 : 1. */
export function LedgerAlpsLogo({
  className = '',
  alt = 'LedgerAlps',
}: {
  className?: string
  alt?: string
}) {
  return <img src="/ledgeralps-logo.svg" alt={alt} className={className} />
}

/**
 * Le logotype sur sa plaque claire — la forme à employer sur fond sombre.
 *
 * La plaque n'est pas décorative : sans elle, un logo bleu nuit sur un fond
 * bleu nuit ne se voit pas.
 */
export function LedgerAlpsPlaque({
  className = '',
  hauteur = 'h-4',
}: {
  className?: string
  hauteur?: string
}) {
  return (
    <span className={`inline-flex items-center rounded bg-white px-2 py-1 ${className}`}>
      <LedgerAlpsLogo className={`${hauteur} w-auto`} />
    </span>
  )
}
