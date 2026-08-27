#!/usr/bin/env python3
"""Detect changes in the authoritative sources LedgerAlps compliance depends on.

This script only detects *change*. It never decides what a change means, and it
never writes an advisory: a legal or specification change is interpreted by a
human, who cites the source. Automating "the law now says X" would put
fabricated legal advice in front of accountants.

Exit codes:
  0  no change (or --update completed)
  1  at least one source changed — CI opens an issue for review
  2  at least one source could not be checked (network/parse failure)

A failure is deliberately NOT reported as a change: a DNS blip must never look
like an amendment to the Data Protection Act.

Usage:
  python scripts/compliance_watch.py            # check, human-readable
  python scripts/compliance_watch.py --json     # check, machine-readable
  python scripts/compliance_watch.py --update   # record current fingerprints
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import sys
import urllib.parse
import urllib.request

HERE = os.path.dirname(os.path.abspath(__file__))
SOURCES_PATH = os.path.join(HERE, "..", "compliance", "sources.json")
SPARQL_ENDPOINT = "https://fedlex.data.admin.ch/sparqlendpoint"
UA = {"User-Agent": "LedgerAlps-compliance-watch/1.0 (+https://github.com/kmdn-ch/LedgerAlps)"}
TIMEOUT = 90

# Latest consolidation date for a given SR (RS) number. dateApplicability is the
# date from which a consolidated text applies, so a new value means the act was
# amended — exactly the signal we want.
FEDLEX_QUERY = """
PREFIX jolux: <http://data.legilux.public.lu/resource/ontology/jolux#>
PREFIX skos: <http://www.w3.org/2004/02/skos/core#>
SELECT ?dateApplicability WHERE {
  ?c a jolux:Consolidation ;
     jolux:dateApplicability ?dateApplicability ;
     jolux:isMemberOf ?act .
  ?act jolux:classifiedByTaxonomyEntry ?e .
  ?e skos:notation ?sr .
  FILTER(str(?sr) = "%s")
} ORDER BY DESC(?dateApplicability) LIMIT 1
"""


class CheckError(Exception):
    """The source could not be checked. Not the same as 'the source changed'."""


class _HTTPSSeulement(urllib.request.HTTPRedirectHandler):
    """Suit les redirections, mais jamais hors de https.

    La garde de schema ci-dessous ne porte que sur l'URL DE DEPART.
    urlopen suit ensuite les redirections avec son gestionnaire par defaut,
    qui accepte http, https et ftp : une source qui redirige vers http://
    serait suivie en clair, et la garde n'aurait ferme que la porte
    d'entree.
    """

    def redirect_request(self, req, fp, code, msg, headers, newurl):
        if urllib.parse.urlparse(newurl).scheme != "https":
            raise CheckError(f"redirection refusee vers {newurl!r} -- https uniquement")
        return super().redirect_request(req, fp, code, msg, headers, newurl)


_OUVREUR = urllib.request.build_opener(_HTTPSSeulement())


def _get(url: str, headers: dict | None = None) -> bytes:
    # Le registre est un fichier suivi, donc de confiance -- mais urlopen
    # accepte file:// et ftp://, et une entree malformee lirait alors un
    # fichier local au lieu d'une source de droit. Defense en profondeur :
    # la garde coute deux lignes et ferme la classe entiere.
    if urllib.parse.urlparse(url).scheme != "https":
        raise CheckError(f"schema d'URL refuse : {url!r} -- https uniquement")

    req = urllib.request.Request(url, headers={**UA, **(headers or {})})
    with _OUVREUR.open(req, timeout=TIMEOUT) as r:
        body = r.read()
        # Throttling has to be named as throttling. EUR-Lex answers 202 with an
        # empty body when it is rate-limiting; that used to reach the regex,
        # match nothing, and be reported as "page structure may have changed" —
        # sending a maintainer to hunt for an edit that never happened.
        if r.status != 200:
            raise CheckError(
                f"HTTP {r.status} with {len(body)} bytes — the source is likely throttling, not changed")
        if not body:
            raise CheckError("empty response body — the source is likely throttling, not changed")
        return body


def fetch_fedlex(source: dict) -> str:
    sr = source["sr_number"]
    url = SPARQL_ENDPOINT + "?" + urllib.parse.urlencode({"query": FEDLEX_QUERY % sr})
    try:
        raw = _get(url, {"Accept": "application/sparql-results+json"})
        rows = json.loads(raw.decode("utf-8"))["results"]["bindings"]
    except CheckError:
        raise  # already carries a precise message
    except Exception as e:  # noqa: BLE001 - any failure is a check failure
        raise CheckError(f"{type(e).__name__}: {e}") from e
    if not rows:
        raise CheckError(f"no consolidation found for RS {sr}")
    return rows[0]["dateApplicability"]["value"]


def fetch_sha256(source: dict) -> str:
    try:
        body = _get(source["url"])
    except CheckError:
        raise
    except Exception as e:  # noqa: BLE001
        raise CheckError(f"{type(e).__name__}: {e}") from e
    # Short digest keeps the registry readable; 16 hex chars is ample to detect
    # an edit, and this is change detection, not a security boundary.
    return hashlib.sha256(body).hexdigest()[:16]


def fetch_links(source: dict) -> str:
    """Highest version number matching link_pattern on the page."""
    try:
        body = _get(source["url"]).decode("utf-8", errors="replace")
    except CheckError:
        raise
    except Exception as e:  # noqa: BLE001
        raise CheckError(f"{type(e).__name__}: {e}") from e
    matches = re.findall(source["link_pattern"], body)
    if not matches:
        raise CheckError("link_pattern matched nothing — page structure may have changed")

    # Dotted numbers are compared numerically so 2.10 ranks above 2.9. Anything
    # else falls back to string order rather than raising: a pattern capturing a
    # hostname or a word used to crash int() and take the whole run down with
    # it, so one awkward registry entry silently disabled all the others.
    def sort_key(v: str):
        parts = v.split(".")
        if all(p.isdigit() for p in parts):
            return (1, tuple(int(p) for p in parts), "")
        return (0, (), v)

    return max(matches, key=sort_key)


FETCHERS = {
    "fedlex_sparql": fetch_fedlex,
    "http_sha256": fetch_sha256,
    "http_links": fetch_links,
}


def check_all(registry: dict) -> tuple[list[dict], list[dict]]:
    changes, failures = [], []
    for source in registry["sources"]:
        fetcher = FETCHERS.get(source["kind"])
        if fetcher is None:
            failures.append({"id": source["id"], "error": f"unknown kind {source['kind']!r}"})
            continue
        try:
            current = fetcher(source)
        except CheckError as e:
            failures.append({"id": source["id"], "name": source["name"], "error": str(e)})
            continue
        if current != source.get("last_seen"):
            changes.append({
                "id": source["id"],
                "name": source["name"],
                "domain": source["domain"],
                "reliability": source["reliability"],
                "previous": source.get("last_seen"),
                "current": current,
                "url": source.get("human_url") or source.get("url"),
                # Les articles dont LedgerAlps dépend, pour que l'alerte dise QUOI
                # relire. « La nLPD a changé » n'oriente personne : l'acte compte
                # des dizaines d'articles et un seul a bougé.
                "watched_articles": source.get("watched_articles", []),
            })
        source["_current"] = current
    return changes, failures


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--update", action="store_true", help="record current fingerprints")
    ap.add_argument("--json", action="store_true", help="machine-readable output")
    args = ap.parse_args()

    with open(SOURCES_PATH, encoding="utf-8") as f:
        registry = json.load(f)

    changes, failures = check_all(registry)

    if args.update:
        for source in registry["sources"]:
            if "_current" in source:
                source["last_seen"] = source.pop("_current")
            source.pop("_current", None)
        with open(SOURCES_PATH, "w", encoding="utf-8") as f:
            json.dump(registry, f, indent=2, ensure_ascii=False)
            f.write("\n")
        print(f"updated {len(changes)} fingerprint(s) in compliance/sources.json")
        return 0

    for source in registry["sources"]:
        source.pop("_current", None)

    if args.json:
        print(json.dumps({"changes": changes, "failures": failures}, indent=2, ensure_ascii=False))
    else:
        if not changes and not failures:
            print(f"No change across {len(registry['sources'])} monitored sources.")
        for c in changes:
            print(f"CHANGED  [{c['reliability']}] {c['name']}")
            print(f"         {c['previous']} -> {c['current']}")
            print(f"         {c['url']}")
        for f_ in failures:
            print(f"FAILED   {f_.get('name', f_['id'])}: {f_['error']}")

    if changes:
        return 1
    if failures:
        return 2
    return 0


if __name__ == "__main__":
    sys.exit(main())
