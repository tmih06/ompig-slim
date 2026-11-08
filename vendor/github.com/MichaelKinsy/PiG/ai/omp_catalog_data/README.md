# OMP catalog snapshot

Source: `@oh-my-pi/pi-catalog@18.14.2` (MIT), matching the installed reference
OMP binary. `manifest.json` records hashes of both upstream inputs and provider
counts. The snapshot contains 5,949 model IDs across all ten OMP kinds. The
provider roster also includes empty discovery providers so login rows remain
visible; Apple is omitted from Linux browser navigation, as in OMP.

Regenerate without running upstream install scripts:

```sh
mkdir -p /tmp/omp-catalog-source
cd /tmp/omp-catalog-source
npm pack @oh-my-pi/pi-catalog@18.14.2 --ignore-scripts
tar -xzf oh-my-pi-pi-catalog-18.14.2.tgz
# Back in this repository:
python3 scripts/import-omp-model-catalog.py \
  --source /tmp/omp-catalog-source/package/src/models.json \
  --version 18.14.2 \
  --output vendor/github.com/MichaelKinsy/PiG/ai/omp_catalog_data
```

Members are deterministic gzip streams, loaded independently per provider. A
small `metric-aliases.json.gz` index records shared public score observations;
OAuth/account-authoritative providers remain provider-local. Its hash is stored
in the manifest, and it is never listed as a provider. This
avoids parsing every provider during an exact-model startup. Provider credential
names come from the compiled rules, never guessed from provider IDs.

For OMP-owned providers the imported chat rows replace the old Pi catalog,
including obsolete entries and invented OpenRouter routing variants. Explicit
user/extension models and authenticated native-only providers remain available.
Native image/classifier implementations are retained behind their canonical
catalog identities. Other kinds are catalog descriptors, not fabricated chat
backends; listing a runner does not install weights or add its transport.

Premium long-context models use OMP's default standard-pricing window; the raw
record retains the full advertised limit and tier rates. Public intelligence/
throughput metadata remains refreshable rather than becoming a user override.
The metadata service reads OMP\'s `models.json.zstd` endpoint and also accepts
plain JSON mirrors. Provider/variant identity remains significant: a `pro`
model never borrows the base model\'s score.
