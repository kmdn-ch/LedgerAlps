# La marque LedgerAlps

Ce dossier porte les fichiers **officiels**, tels qu'ils ont été fournis, et ce
qui en dérive mécaniquement. Rien n'y est redessiné.

| Fichier | Ce que c'est |
|---|---|
| `LOGO.svg` | Le logotype « LedgerAlps ». **Original, intact.** |
| `icon.svg` | Le monogramme « LA ». **Original, intact.** C'est l'icône du bureau. |
| `ledgeralps.ico` | L'icône Windows, sept tailles, produite depuis `icon.svg`. |
| `faire_ico.py` | Assemble le `.ico` à partir des rendus PNG. |
| `faire_syso.py` | Produit la ressource Windows qui donne son icône à l'exécutable. |

## Pourquoi les originaux ne bougent pas

Une première version de la marque avait été **reconstruite** à partir de
captures d'écran. Elle perdait ce qui fait un logo : la police, l'espacement, le
détail des raccords. Les fichiers ci-dessus sont donc la référence, et tout ce
que le produit affiche en dérive sans intervention manuelle.

## Ce que le produit utilise, et d'où ça vient

**Dans l'interface** — `frontend/public/ledgeralps-logo.svg` et
`ledgeralps-icon.svg`. Ce sont les mêmes tracés, recopiés à l'identique. Deux
choses seulement changent, et aucune ne touche au dessin :

- le `viewBox` cadre sur les limites réelles du dessin, au lieu de la planche de
  1408 × 768 qui l'entoure de blanc ;
- le rectangle de fond blanc est retiré, pour que la marque se pose sur
  n'importe quel support.

Les polices sont **déjà vectorisées** dans les originaux : l'espacement et le
style sont dans les coordonnées, et rien ne les altère.

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
