# Workspace image

Each release also publishes a small multi-architecture workspace image with `ompig-slim`, `git`, `fd`,
`rg`, `jq`, `curl`, SSH, Python, and common archive/patch tools. It is intentionally runtime-only; add
language toolchains in a project-specific image when needed.

```sh
docker run --rm -it -v "$PWD:/workspace" \
  ghcr.io/tmih06/ompig-slim-workspace:<version>
```
