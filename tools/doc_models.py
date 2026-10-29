#!/usr/bin/env python3
"""Guard against dead model IDs in the documentation.

Every model ID that a user can copy out of our documentation must be priced AND
available in the public catalog. A dead ID fails the admission gate with
``400 model pricing is not available`` before any upstream call, so a copied
example simply does not work — that is what ``tools/model-allowlist.json`` and
this script prevent.

Surfaces scanned (user-facing examples only — ``CLAUDE.md`` is an internal
agent doc and is deliberately excluded):

    api-examples/**                 public examples + integrations repo
    README.md, README_ASH.md        gateway READMEs
    frontend/src/utils/apiSnippets.ts          SNIPPET_MODELS
    frontend/src/views/user/DocsView.vue       in-product /docs code samples

Usage:
    python3 tools/doc_models.py --check     # offline, CI-safe (also `make check-doc-models`)
    python3 tools/doc_models.py --refresh   # network: re-verify against the live
                                            # catalog and rewrite the allowlist

``--check`` fails when the set of documented model IDs differs from the
allowlist in either direction (a documented model that is not verified, or a
verified entry that nothing references any more).

``--refresh`` queries ``GET /api/v1/models``, rewrites the allowlist with the
documented IDs that are priced and available right now, and fails (exit 1)
while any documented ID is still dead — so a scheduled run reports drift
instead of hiding it. Run it after adding/renaming a model in the docs, or on a
schedule so the housekeeping stays current.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import sys
import urllib.request
from datetime import datetime, timezone

REPO_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
ALLOWLIST_PATH = os.path.join(REPO_ROOT, "tools", "model-allowlist.json")
CATALOG_URL = os.environ.get(
    "BESTAIPRICE_CATALOG_URL", "https://bestaiprice.com/gateway/api/v1/models"
)

SCAN_TARGETS = [
    "api-examples",
    "README.md",
    "README_ASH.md",
    "frontend/src/utils/apiSnippets.ts",
    "frontend/src/views/user/DocsView.vue",
]
SCAN_EXTENSIONS = (".md", ".sh", ".py", ".mjs", ".js", ".ts", ".vue", ".json", ".yaml", ".yml", ".env")
SKIP_DIRS = {".git", "node_modules", "dist", "build", "__pycache__"}

# Words that look like a model ID but are not one (file names, product names).
NOT_MODELS = {"claude-code", "claude-relay-service", "claude-code-skill.md"}

MODEL_RE = re.compile(
    r"\b(?:"
    r"gpt-[0-9][0-9a-zA-Z.\-]*"
    r"|o[13](?:-mini)?"
    r"|claude-[0-9a-zA-Z.\-]+"
    r"|gemini-[0-9][0-9a-zA-Z.\-]*"
    r"|deepseek-[a-z0-9.\-]+"
    r"|kimi-[a-z0-9.\-]+"
    r"|qwen[0-9a-z.\-]*"
    r"|grok-[0-9a-z.\-]+"
    r")\b"
)


def iter_scan_files() -> list[str]:
    files: list[str] = []
    for target in SCAN_TARGETS:
        path = os.path.join(REPO_ROOT, target)
        if os.path.isfile(path):
            files.append(path)
            continue
        for dirpath, dirnames, filenames in os.walk(path):
            dirnames[:] = [d for d in dirnames if d not in SKIP_DIRS]
            for name in filenames:
                if name.lower().endswith(SCAN_EXTENSIONS):
                    files.append(os.path.join(dirpath, name))
    return files


def documented_models() -> dict[str, set[str]]:
    """model id -> files that reference it."""
    found: dict[str, set[str]] = {}
    for path in iter_scan_files():
        try:
            text = open(path, encoding="utf-8", errors="ignore").read()
        except OSError:
            continue
        rel = os.path.relpath(path, REPO_ROOT)
        for token in set(MODEL_RE.findall(text)):
            lowered = token.lower()
            if lowered in NOT_MODELS or not any(ch.isdigit() for ch in lowered):
                continue
            found.setdefault(lowered, set()).add(rel)
    return found


def load_allowlist() -> dict:
    try:
        with open(ALLOWLIST_PATH, encoding="utf-8") as fh:
            data = json.load(fh)
    except FileNotFoundError:
        return {"generated_at": None, "models": []}
    except json.JSONDecodeError as exc:
        sys.exit(f"ERROR: {os.path.relpath(ALLOWLIST_PATH, REPO_ROOT)} is not valid JSON: {exc}")
    data.setdefault("models", [])
    return data


def fetch_catalog() -> dict[str, dict]:
    """model id (canonical stem and full id) -> catalog record."""
    request = urllib.request.Request(CATALOG_URL, headers={"User-Agent": "doc-model-check/1.0"})
    with urllib.request.urlopen(request, timeout=45) as response:
        payload = json.load(response)
    index: dict[str, dict] = {}
    for item in payload.get("data", []):
        canonical = item.get("canonical_model_id") or ""
        stem = canonical.split("/", 1)[-1].lower() if "/" in canonical else canonical.lower()
        priced = item.get("input_price_per_1m") is not None or item.get("native_price_usd") is not None
        record = {
            "available": bool(priced and item.get("available")),
            "status": item.get("status"),
        }
        for key in {stem, canonical.lower()}:
            if not key:
                continue
            previous = index.get(key)
            if previous is None or (record["available"] and not previous["available"]):
                index[key] = record
    return index


def describe(mapping: dict[str, set[str]]) -> str:
    lines = []
    for model in sorted(mapping):
        files = sorted(mapping[model])[:3]
        more = "" if len(mapping[model]) <= 3 else f" (+{len(mapping[model]) - 3} more)"
        lines.append(f"    {model:24} {', '.join(files)}{more}")
    return "\n".join(lines)


def check() -> int:
    allow = load_allowlist()
    verified = {str(m).lower() for m in allow["models"]}
    refs = documented_models()

    missing = {m: refs[m] for m in refs if m not in verified}
    stale = {m: {"tools/model-allowlist.json"} for m in verified if m not in refs}

    if not missing and not stale:
        print(f"OK: all {len(refs)} documented model IDs are in tools/model-allowlist.json")
        return 0

    if missing:
        print("FAIL: documented model IDs that are NOT in the verified allowlist:")
        print(describe(missing))
        print("      -> re-verify the ID against the live catalog and refresh the list:")
        print("         python3 tools/doc_models.py --refresh")
    if stale:
        print("FAIL: allowlist entries that nothing references any more:")
        print(describe(stale))
        print("      -> drop them (python3 tools/doc_models.py --refresh) or use them in the docs")
    print(f"      allowlist generated_at: {allow.get('generated_at')}")
    return 1


def refresh() -> int:
    refs = documented_models()
    catalog = fetch_catalog()

    alive: list[str] = []
    dead: dict[str, set[str]] = {}
    for model in sorted(refs):
        record = catalog.get(model)
        if record is not None and record["available"]:
            alive.append(model)
        else:
            dead[model] = refs[model]

    payload = {
        "note": (
            "Model IDs used by the user-facing docs/examples. Every entry was priced AND "
            "available in the public catalog when generated; refresh with "
            "`python3 tools/doc_models.py --refresh`, verify with `make check-doc-models`."
        ),
        "generated_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "catalog_url": CATALOG_URL,
        "models": alive,
    }
    previous = load_allowlist().get("models", [])
    os.makedirs(os.path.dirname(ALLOWLIST_PATH), exist_ok=True)
    with open(ALLOWLIST_PATH, "w", encoding="utf-8") as fh:
        json.dump(payload, fh, indent=2, ensure_ascii=False)
        fh.write("\n")

    if previous != alive:
        added = sorted(set(alive) - set(previous))
        removed = sorted(set(previous) - set(alive))
        print(f"allowlist refreshed ({os.path.relpath(ALLOWLIST_PATH, REPO_ROOT)})")
        if added:
            print(f"  added:   {', '.join(added)}")
        if removed:
            print(f"  removed: {', '.join(removed)}")
    else:
        print(f"allowlist unchanged ({len(alive)} documented model IDs verified)")

    if dead:
        print("DRIFT: these documented model IDs are NOT priced/available — fix the docs:")
        print(describe(dead))
        return 1
    return 0


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    group = parser.add_mutually_exclusive_group()
    group.add_argument("--check", action="store_true", help="offline check against the allowlist (default)")
    group.add_argument("--refresh", action="store_true", help="re-verify against the live catalog and rewrite the allowlist")
    args = parser.parse_args()
    if args.refresh:
        return refresh()
    return check()


if __name__ == "__main__":
    sys.exit(main())
