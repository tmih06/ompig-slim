#!/usr/bin/env python3
"""Import OMP's compact bundled catalog without executing its package scripts.

Input is src/models.json from the pinned @oh-my-pi/pi-catalog npm package.
Per-provider gzip members keep native exact-model startup lazy and deterministic.
"""
import argparse
import gzip
import io
import hashlib
import json
import re
from pathlib import Path


def decode_catalog(data):
    """Decode compact catalog v1, including shared compat bases and removals."""
    if data["version"] != 1:
        raise ValueError("unsupported OMP catalog format")
    cache = {}

    def decode(value):
        """Resolve one interned object/array; catalog nodes are immutable DAGs."""
        if not isinstance(value, list):
            return value
        index = value[0]
        if index < 0:
            return None
        if index in cache:
            return cache[index]
        node = data["nodes"][index]
        if node[0] == 1:
            result = [decode(entry) for entry in node[1]]
        else:
            result = {
                data["keys"][node[1][i]]: decode(node[1][i + 1])
                for i in range(0, len(node[1]), 2)
            }
            for i, key in enumerate(data["booleanShapes"][node[2]]):
                result[data["keys"][key]] = bool((node[3][i // 32] >> (i % 32)) & 1)
        cache[index] = result
        return result

    bases = dict(data["compat"])
    providers = {}
    for provider, constants, rows in data["providers"]:
        models = []
        for row in rows:
            model = {"id": row[0], **decode(constants), **decode(row[1])}
            if len(row) > 2:
                removed, changed = row[2]
                compat = dict(decode(bases[model["api"]]))
                for key in removed:
                    compat.pop(data["keys"][key], None)
                compat.update(decode(changed))
                model["compat"] = compat
            models.append(model)
        providers[provider] = models
    return providers


def gzip_bytes(payload):
    """Return a gzip member with no timestamp or inherited source filename."""
    buffer = io.BytesIO()
    with gzip.GzipFile(filename="", fileobj=buffer, mode="wb", mtime=0) as archive:
        archive.write(payload)
    return buffer.getvalue()


def main():
    """Write reproducible provider snapshots and their provenance manifest."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", type=Path, required=True)
    parser.add_argument("--version", required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    raw = args.source.read_bytes()
    providers = decode_catalog(json.loads(raw))
    rules_raw = (args.source.parent / "compat" / "rules.json").read_bytes()
    compiled = json.loads(rules_raw)["providers"]
    for provider in compiled:
        providers.setdefault(provider, [])
    # Public-source aliases are transport configuration, not model identities.
    # Read the canonical descriptor arguments so Together/W&B/etc. keep using
    # OMP's source keys even when they differ from the runtime provider ID.
    descriptor_raw = (
        args.source.parent / "provider-models" / "openai-compat.ts"
    ).read_text()
    aliases = {}
    for match in re.finditer(
        r'(?:simpleModelsDevDescriptor|openAiCompletionsDescriptor|openAiResponsesDescriptor|anthropicMessagesDescriptor)\(\s*"([^"]+)"\s*,\s*"([^"]+)"',
        descriptor_raw,
    ):
        source_key, provider = match.groups()
        aliases.setdefault(provider, []).append(source_key)
    policies = {}
    for provider, entry in compiled.items():
        policies[provider] = {
            "defaultModel": entry.get("defaultModel", ""),
            "sourceKeys": list(dict.fromkeys([provider, *aliases.get(provider, [])])),
            "accountScoped": bool(
                entry.get("dynamicModelsAuthoritative")
                or entry.get("discovery", {}).get("oauthProvider")
            ),
        }
    args.output.mkdir(parents=True, exist_ok=True)
    for previous in args.output.glob("*.json.gz"):
        if previous.name.removesuffix(".json.gz") not in providers:
            previous.unlink()
    for provider, models in providers.items():
        if "/" in provider or ".." in provider:
            raise ValueError("unsafe provider key")
        encoded = json.dumps(models, ensure_ascii=False, separators=(",", ":")).encode()
        (args.output / f"{provider}.json.gz").write_bytes(gzip_bytes(encoded))
    metric_aliases = {}
    for provider in sorted(providers):
        # OAuth-discovery rows are account-scoped observations, not public
        # cross-provider references. They remain in their own provider catalog.
        entry = compiled.get(provider, {})
        if entry.get("discovery", {}).get("oauthProvider") or entry.get(
            "dynamicModelsAuthoritative"
        ):
            continue
        for model in sorted(providers[provider], key=lambda item: item["id"]):
            if model.get("type", "chat") != "chat" or (
                "int" not in model and "tps" not in model
            ):
                continue
            bare_id = model["id"].rsplit("/", 1)[-1].lower()
            candidate = {"provider": provider, "id": model["id"]}
            for field in ("int", "tps"):
                if field in model:
                    candidate[field] = model[field]
            metric_aliases.setdefault(bare_id, []).append(candidate)
    metrics_raw = json.dumps(
        metric_aliases, ensure_ascii=False, separators=(",", ":")
    ).encode()
    (args.output / "metric-aliases.json.gz").write_bytes(gzip_bytes(metrics_raw))
    (args.output / "provider-policy.json").write_text(
        json.dumps(policies, indent=2) + "\n"
    )
    auth = {
        provider: entry["envVars"]
        for provider, entry in compiled.items()
        if entry.get("envVars")
    }
    (args.output / "provider-auth.json").write_text(json.dumps(auth, indent=2) + "\n")
    manifest = {
        "package": "@oh-my-pi/pi-catalog",
        "version": args.version,
        "sourceSha256": hashlib.sha256(raw).hexdigest(),
        "rulesSha256": hashlib.sha256(rules_raw).hexdigest(),
        "metricAliasesSha256": hashlib.sha256(metrics_raw).hexdigest(),
        "discoveryDescriptorsSha256": hashlib.sha256(
            descriptor_raw.encode()
        ).hexdigest(),
        "providers": {provider: len(models) for provider, models in providers.items()},
    }
    (args.output / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
    print(
        f"Imported {sum(map(len, providers.values()))} models from {len(providers)} providers"
    )


if __name__ == "__main__":
    main()
