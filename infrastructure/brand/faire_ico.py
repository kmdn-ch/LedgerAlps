# -*- coding: utf-8 -*-
"""Assemble un .ico Windows à partir des PNG rendus depuis icon.svg.

# Le format, et le choix qui compte

Un .ico est un conteneur : un entête, un répertoire d'entrées, puis les images.
Chaque image est soit un DIB (bitmap sans son entête de fichier, avec un masque
de transparence collé dessous), soit un PNG entier.

Windows Vista et suivants lisent le PNG à toutes les tailles. Les petites
tailles restent néanmoins en DIB : c'est ce que fait Windows lui-même, et
certains chemins anciens de l'explorateur — l'aperçu d'un raccourci, une boîte
de dialogue héritée — ne savent lire que cela. Le coût est quelques kilooctets.

# Pourquoi pas d'agrandissement

Chaque taille est rendue depuis le SVG, jamais rééchantillonnée depuis une
autre : agrandir un 32 px en 256 px donnerait une bouillie là où le vecteur
donne un trait net.
"""
import io
import os
import struct
import sys
import zlib

sys.stdout.reconfigure(encoding="utf-8")

ICO = os.path.join(os.path.dirname(os.path.abspath(__file__)), "ico")
TAILLES = [16, 24, 32, 48, 64, 128, 256]
EN_PNG = {128, 256}  # au-delà de 64 px, le PNG évite un .ico obèse


def lire_png(chemin):
    """Rend (largeur, hauteur, pixels RGBA) d'un PNG écrit par un navigateur."""
    with io.open(chemin, "rb") as f:
        brut = f.read()
    if brut[:8] != b"\x89PNG\r\n\x1a\n":
        raise SystemExit(f"{chemin} n'est pas un PNG")
    pos, larg, haut, idat, profondeur, couleur = 8, 0, 0, b"", 0, 0
    while pos < len(brut):
        (taille,) = struct.unpack(">I", brut[pos:pos + 4])
        typ = brut[pos + 4:pos + 8]
        corps = brut[pos + 8:pos + 8 + taille]
        if typ == b"IHDR":
            larg, haut, profondeur, couleur = struct.unpack(">IIBB", corps[:10])
        elif typ == b"IDAT":
            idat += corps
        pos += 12 + taille
    if profondeur != 8 or couleur != 6:
        raise SystemExit(f"{chemin} : attendu RGBA 8 bits, reçu profondeur={profondeur} couleur={couleur}")

    flux = zlib.decompress(idat)
    pas = larg * 4
    sortie = bytearray()
    precedente = bytearray(pas)
    i = 0
    for _ in range(haut):
        filtre = flux[i]
        i += 1
        ligne = bytearray(flux[i:i + pas])
        i += pas
        # Les cinq filtres du PNG. Un navigateur en emploie plusieurs selon la
        # ligne ; les ignorer donnerait une image bruitée, pas une erreur.
        for x in range(pas):
            a = ligne[x - 4] if x >= 4 else 0
            b = precedente[x]
            c = precedente[x - 4] if x >= 4 else 0
            if filtre == 1:
                ligne[x] = (ligne[x] + a) & 0xFF
            elif filtre == 2:
                ligne[x] = (ligne[x] + b) & 0xFF
            elif filtre == 3:
                ligne[x] = (ligne[x] + (a + b) // 2) & 0xFF
            elif filtre == 4:
                p = a + b - c
                pa, pb, pc = abs(p - a), abs(p - b), abs(p - c)
                pr = a if (pa <= pb and pa <= pc) else (b if pb <= pc else c)
                ligne[x] = (ligne[x] + pr) & 0xFF
        sortie += ligne
        precedente = ligne
    return larg, haut, bytes(sortie)


def dib(larg, haut, rgba):
    """Un DIB 32 bits, lignes de bas en haut, plus son masque AND."""
    entete = struct.pack("<IiiHHIIiiII", 40, larg, haut * 2, 1, 32, 0, 0, 0, 0, 0, 0)
    couleurs = bytearray()
    for y in range(haut - 1, -1, -1):          # le DIB se lit du bas vers le haut
        for x in range(larg):
            r, v, b, a = rgba[(y * larg + x) * 4:(y * larg + x) * 4 + 4]
            couleurs += bytes((b, v, r, a))    # BGRA
    # Le masque AND : obsolète pour une image 32 bits, mais sa PLACE est
    # obligatoire. L'omettre donne des icônes tronquées dans certains chemins.
    pas_masque = ((larg + 31) // 32) * 4
    masque = bytes(pas_masque * haut)
    return bytes(entete) + bytes(couleurs) + masque


images = []
for t in TAILLES:
    chemin = os.path.join(ICO, f"icon-{t}.png")
    if t in EN_PNG:
        with io.open(chemin, "rb") as f:
            images.append((t, f.read()))
    else:
        l, h, px = lire_png(chemin)
        if (l, h) != (t, t):
            raise SystemExit(f"{chemin} fait {l}x{h}, attendu {t}x{t}")
        images.append((t, dib(l, h, px)))

# ICONDIR + ICONDIRENTRY x n + les images
decalage = 6 + 16 * len(images)
entete = struct.pack("<HHH", 0, 1, len(images))
repertoire = b""
corps = b""
for t, data in images:
    octet = 0 if t >= 256 else t          # 0 veut dire 256
    repertoire += struct.pack("<BBBBHHII", octet, octet, 0, 0, 1, 32, len(data), decalage)
    corps += data
    decalage += len(data)

sortie = os.path.join(ICO, "ledgeralps.ico")
with io.open(sortie, "wb") as f:
    f.write(entete + repertoire + corps)
print(f"{sortie} : {len(images)} tailles, {len(entete + repertoire + corps)} octets")
for t, d in images:
    print(f"  {t:>3} px — {'PNG' if t in EN_PNG else 'DIB'} — {len(d)} o")
