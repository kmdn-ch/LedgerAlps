# -*- coding: utf-8 -*-
"""Fabrique le .syso qui donne son icône à ledgeralps.exe.

# Pourquoi un fichier objet et pas un outil

Go n'a pas de directive pour poser une icône : il ramasse simplement les
fichiers `.syso` du répertoire du paquet et les donne à l'éditeur de liens. Ce
sont des fichiers objets COFF. Les outils habituels — `rsrc`, `goversioninfo` —
les produisent, mais ils se récupèrent sur le réseau, et LedgerAlps se construit
hors ligne. Ce script fait la même chose, avec le format écrit noir sur blanc.

# Ce que contient le fichier

Une section `.rsrc` portant l'arbre de ressources d'un exécutable Windows :

    racine
      ├── RT_ICON (3)        une entrée par taille d'image
      └── RT_GROUP_ICON (14) le répertoire qui les rassemble

# Le point qui casse tout si on l'oublie

Chaque feuille de l'arbre (IMAGE_RESOURCE_DATA_ENTRY) pointe sur ses octets par
une ADRESSE VIRTUELLE, que seul l'éditeur de liens connaît. On écrit donc un
décalage relatif à la section, ET une relocalisation `IMAGE_REL_AMD64_ADDR32NB`
sur ce champ : l'éditeur y ajoutera l'adresse de la section. Sans ces
relocalisations, l'exécutable se construit sans broncher et Windows n'affiche
aucune icône — la ressource pointe dans le vide.
"""
import io
import os
import struct
import sys

sys.stdout.reconfigure(encoding="utf-8")

ICO = os.path.join(os.path.dirname(os.path.abspath(__file__)), "ico")
SOURCE = os.path.join(ICO, "ledgeralps.ico")
SORTIE = os.path.join(ICO, "rsrc_windows_amd64.syso")

RT_ICON, RT_GROUP_ICON = 3, 14
LANGUE = 0x0409  # anglais (US) — la langue de la RESSOURCE, pas de l'interface

# ── Lire le .ico ─────────────────────────────────────────────────────────────
b = io.open(SOURCE, "rb").read()
_, typ, nb = struct.unpack("<HHH", b[:6])
if typ != 1:
    raise SystemExit("ce fichier n'est pas un .ico")

images, entrees = [], []
for i in range(nb):
    o = 6 + 16 * i
    w, h, c, r, pl, bc, taille, dec = struct.unpack("<BBBBHHII", b[o:o + 16])
    images.append(b[dec:dec + taille])
    # L'entrée de GROUPE reprend les mêmes champs, sauf le décalage remplacé
    # par l'IDENTIFIANT de la RT_ICON correspondante.
    entrees.append((w, h, c, r, pl, bc, taille, i + 1))

# GRPICONDIR : ce que Windows lit pour savoir quelles tailles existent.
groupe = struct.pack("<HHH", 0, 1, nb)
for (w, h, c, r, pl, bc, taille, ident) in entrees:
    groupe += struct.pack("<BBBBHHIH", w, h, c, r, pl, bc, taille, ident)

ressources = [(RT_ICON, i + 1, d) for i, d in enumerate(images)]
ressources.append((RT_GROUP_ICON, 1, groupe))

# ── Poser l'arbre ────────────────────────────────────────────────────────────
DIR, ENT, DATA = 16, 8, 16
par_type = {}
for t, ident, d in ressources:
    par_type.setdefault(t, []).append((ident, d))
types = sorted(par_type)

pos = DIR + ENT * len(types)                     # après la racine
dec_type = {}
for t in types:
    dec_type[t] = pos
    pos += DIR + ENT * len(par_type[t])
dec_langue = {}
for t in types:
    for ident, _ in par_type[t]:
        dec_langue[(t, ident)] = pos
        pos += DIR + ENT                          # un répertoire de langue
dec_data = {}
for t in types:
    for ident, _ in par_type[t]:
        dec_data[(t, ident)] = pos
        pos += DATA
octets_debut = pos

dec_octets, corps = {}, b""
for t in types:
    for ident, d in par_type[t]:
        dec_octets[(t, ident)] = octets_debut + len(corps)
        corps += d
        corps += b"\x00" * ((4 - len(d) % 4) % 4)  # aligné sur 4

# ── Écrire la section ────────────────────────────────────────────────────────
s = struct.pack("<IIHHHH", 0, 0, 0, 0, 0, len(types))
for t in types:
    s += struct.pack("<II", t, dec_type[t] | 0x80000000)
for t in types:
    s += struct.pack("<IIHHHH", 0, 0, 0, 0, 0, len(par_type[t]))
    for ident, _ in par_type[t]:
        s += struct.pack("<II", ident, dec_langue[(t, ident)] | 0x80000000)
for t in types:
    for ident, _ in par_type[t]:
        s += struct.pack("<IIHHHH", 0, 0, 0, 0, 0, 1)
        s += struct.pack("<II", LANGUE, dec_data[(t, ident)])

relocalisations = []
for t in types:
    for ident, d in par_type[t]:
        relocalisations.append(len(s))            # le champ OffsetToData
        s += struct.pack("<IIII", dec_octets[(t, ident)], len(d), 0, 0)
if len(s) != octets_debut:
    raise SystemExit(f"arbre mal posé : {len(s)} != {octets_debut}")
s += corps

# ── L'enveloppe COFF ─────────────────────────────────────────────────────────
NB_SYMBOLES = 1
TAILLE_RELOC = 10
debut_section = 20 + 40
debut_reloc = debut_section + len(s)
debut_symboles = debut_reloc + TAILLE_RELOC * len(relocalisations)

entete = struct.pack("<HHIIIHH",
                     0x8664,                      # AMD64
                     1,                           # une section
                     0,
                     debut_symboles,
                     NB_SYMBOLES,
                     0,
                     0)
section = struct.pack("<8sIIIIIIHHI",
                      b".rsrc\0\0\0",
                      0, 0,
                      len(s), debut_section,
                      debut_reloc, 0,
                      len(relocalisations), 0,
                      0x40300040)                 # données init. + lecture + align 4

reloc = b""
for adresse in relocalisations:
    reloc += struct.pack("<IIH", adresse, 0, 3)   # 3 = IMAGE_REL_AMD64_ADDR32NB

# Le symbole que visent les relocalisations : la section elle-même.
symboles = struct.pack("<8sIhHBB", b".rsrc\0\0\0", 0, 1, 0, 3, 0)  # classe 3 = STATIC
chaines = struct.pack("<I", 4)                    # table de chaînes vide

io.open(SORTIE, "wb").write(entete + section + s + reloc + symboles + chaines)
print(f"{SORTIE}")
print(f"  {len(images)} images, {len(relocalisations)} relocalisations, "
      f"{os.path.getsize(SORTIE)} octets")
