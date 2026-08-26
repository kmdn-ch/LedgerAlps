# La marque LedgerAlps

Ce dossier porte les fichiers **officiels**, tels qu'ils ont été fournis, et ce
qui en dérive mécaniquement. Rien n'y est redessiné.

| Fichier | Ce que c'est |
|---|---|
| `LOGO.png` | Le logotype « LedgerAlps ». **Fourni, intact.** C'est la référence. |
| `LOGO.svg` | Le même, vectorisé depuis `LOGO.png`. C'est ce que l'interface affiche. |
| `icon.svg` | Le monogramme « LA ». **Original, intact.** C'est l'icône du bureau. |
| `ledgeralps.ico` | L'icône Windows, sept tailles, produite depuis `icon.svg`. |
| `faire_ico.py` | Assemble le `.ico` à partir des rendus PNG. |
| `faire_syso.py` | Produit la ressource Windows qui donne son icône à l'exécutable. |

## Pourquoi on décalque au lieu de retaper

Une première version de la marque avait été **reconstruite** à la police, à
partir de captures d'écran. Elle perdait ce qui fait un logo : la graisse,
l'espacement, le détail des raccords. Les lettres du logotype sont donc
vectorisées depuis l'image fournie, pas ressaisies.

**Sauf le badge suisse**, qui est *construit*. Un drapeau suisse n'est pas un
dessin, c'est une géométrie : l'ordonnance sur les armoiries (RS 232.21) le
décrit sur un carré de 32 unités où chaque bras mesure 6 unités de large et 20
d'un bout à l'autre, centré. Le décalquer revenait à recopier les à-peu-près de
l'image — l'ancien logo portait un rouge `#C42527` là où le `#DA291C` (Pantone
485 C) fait foi, et une croix qui n'était pas tout à fait aux proportions.

Écart mesuré entre `LOGO.png` et `LOGO.svg`, pixel à pixel après rendu : **0,76
sur 255 en moyenne** sur les lettres, deux pixels au-delà du seuil sur 102 600.
Le badge s'en écarte davantage, et c'est voulu : c'est là qu'on a corrigé.

## Ce que le produit utilise, et d'où ça vient

**Dans l'interface** — `frontend/public/ledgeralps-logo.svg` et
`ledgeralps-icon.svg`.

- le LOGOTYPE est la copie exacte de `LOGO.svg`. Son fond est transparent : il
  se pose sur une plaque claire que la page fournit elle-même là où le fond est
  sombre — barre latérale, écran de connexion — et tel quel là où il est clair,
  en haut à droite de l'espace de travail ;
- l'ICÔNE reprend les tracés d'`icon.svg`, avec pour seuls changements un
  `viewBox` cadré sur le dessin au lieu de la planche de 1408 × 768 qui
  l'entoure de blanc. Le fond blanc y RESTE, étendu au cadre carré : une icône
  se pose sur un bureau, une barre des tâches, un fond dont on ne sait rien, et
  un monogramme bleu nuit sur fond transparent y disparaîtrait.

Le monogramme n'a pas suivi la mise à jour du logotype : il porte encore son
rouge `#CA2E24`. C'est un fichier séparé, qui alimente le `.ico` et la ressource
Windows, et le refaire est un chantier à part.

**Sur le bureau et dans le menu Démarrer** — l'icône vient de
`cmd/launcher/rsrc_windows_amd64.syso`, une ressource Windows liée à
`ledgeralps.exe`. Sans elle, Windows affiche son icône générique bleue et rien
ne distingue LedgerAlps d'un exécutable anonyme.

**Dans l'installeur** — `installer.nsi` pointe sur `ledgeralps.ico` par
`MUI_ICON` / `MUI_UNICON`.

## Refaire les fichiers dérivés

Le seul passage qui demande un outil est le rendu du SVG en PNG. Il se fait dans
un navigateur, parce que c'est le seul moteur de rendu SVG dont on soit certain
qu'il est présent — et parce qu'il donne exactement ce que l'utilisateur verra.

1. Ouvrir `icon.svg` dans une page qui retire le fond blanc, cadre le dessin
   dans un CARRÉ (le dessin fait 472,39 × 423,17 : on le centre, avec 7 % de
   marge), et le dessine sur un canevas aux tailles 16, 24, 32, 48, 64, 128
   et 256. Enregistrer les PNG sous `icon-<taille>.png`.
2. `python faire_ico.py` — assemble `ledgeralps.ico`. Les tailles jusqu'à 64 px
   sont en DIB, au-delà en PNG : c'est ce que fait Windows lui-même, et
   certains chemins anciens de l'explorateur ne lisent que le DIB.
3. `python faire_syso.py` — produit `rsrc_windows_amd64.syso`, à copier dans
   `cmd/launcher/`. Le suffixe du nom est porteur : Go ne lie ce fichier que
   pour `GOOS=windows GOARCH=amd64`, et les compilations Linux l'ignorent.

## Comment vérifier que ça a marché

```powershell
Add-Type -AssemblyName System.Drawing
$i = [System.Drawing.Icon]::ExtractAssociatedIcon("ledgeralps.exe")
$b = $i.ToBitmap()
$b.GetPixel([int]($b.Width*0.30), [int]($b.Height*0.55)).Name   # attendu ff1c3656
$b.GetPixel([int]($b.Width*0.86), [int]($b.Height*0.14)).Name   # attendu ffca2e24
```

`ff1c3656` est le bleu du monogramme, `ffca2e24` le rouge du drapeau — les deux
couleurs exactes d'`icon.svg`. Un exécutable sans la ressource rend `ff0c7ccb`,
le bleu de l'icône générique de Windows.
