# Public model metadata reference

`omp-openai-public-metadata.json` contains seven public OpenAI score rows from
OMP's `models-metadata.json` schema (catalog.stencil.so, MIT upstream importer).
The fetched/revalidation timestamps are retained. These are display metrics,
not model definitions, credentials, inference responses, or user settings.

Use it to seed both isolated PTY profiles:

```sh
python3 scripts/compare-model-tuis.py \
  --omp /home/ubuntu/.local/bin/omp --slim ./bin/ompig-slim \
  --output /tmp/omp-catalog-parity --unfiltered --openai \
  --metadata-cache scripts/testdata/omp-openai-public-metadata.json
```

No provider is disabled and no `--models` filter is set. All seven screens are
compared cell-for-cell, including intelligence and throughput values. The
snapshot also contains base luna/terra scores so tests expose an incorrect
attempt to apply them to distinct `pro` variants. The catalog remains the full
5,949-entry OMP snapshot; only public metadata observations are pinned here.
OMP can still refresh its public catalog, so a future upstream score change may
require a reviewed snapshot update. `--catalog-identities` is a separate audit
mode for model identities/order/counts, not a visual-parity gate.
