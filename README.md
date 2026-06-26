# doze-modules

Out-of-process engine **plugins** for [doze](https://github.com/doze-dev/doze).
Each module is a pure-Go plugin binary that speaks doze's engine gRPC protocol;
doze fetches one by engine type, verifies its checksum, caches it under
`~/.doze/modules`, and runs it as a subprocess. This is the Terraform *provider*
model: doze core is a thin host, every engine is a separately-versioned module.

## Layout

```
modules.yaml          # the manifest: each module's source path (in doze) + version
cmd/dzm/              # the build tool: cross-compiles plugins + assembles the release layout
.github/workflows/    # release CI (single runner — plugins are pure-Go cross-compiles)
```

A module is **not** its own repo — it's an entry in `modules.yaml`. Authoring or
upgrading one is a PR editing that file; CI builds the four supported triples
(`{aarch64,x86_64}-{apple-darwin,unknown-linux-gnu}`) and publishes cumulatively.

## How doze consumes it

Each module is published to its own rolling GitHub release (tag = module name)
serving an `index.yaml` + per-platform archives, mirroring `doze-binaries`:

```
releases/download/<module>/index.yaml
releases/download/<module>/<module>-plugin-<version>-<triple>.tar.gz   # contains bin/<module>-plugin
```

doze resolves an engine type → module of the same name, fetches the archive for
the host platform, checks its `sha256`, and caches the plugin. Override the
source for development:

```sh
# a local dist dir (built by dzm) or any mirror base
export DOZE_MODULES_MIRROR=file:///path/to/dist
# or a single engine, straight to a binary, skipping fetch entirely
export DOZE_POSTGRES_PLUGIN=/path/to/postgres-plugin
```

## Building locally

```sh
go build -o /tmp/dzm ./cmd/dzm
/tmp/dzm --doze ../doze --out dist                     # all modules, all triples
/tmp/dzm --doze ../doze --out dist --module valkey     # one module
DOZE_MODULES_MIRROR=file://$PWD/dist doze up            # point doze at it
```
