---
name: transpareo
description: Drive a Transpareo workspace: products, components, brands and Digital Product Passports through the transpareo command line and its MCP server. Use when a task mentions Transpareo, a passport (DPP), a product catalogue on Transpareo, or the transpareo tool.
---

# Transpareo

One binary, `transpareo`, talks to a Transpareo workspace with the
credentials of an API consumer. Everything it prints is JSON when
the output is not a terminal, so read it as data. When the MCP
server is connected, its tools cover every flow below; do not look
for the binary or shell out to it. Use the command line for files
on disk, for scripts that run without an assistant, and when no
server is connected.

A refused call answers the error code, the message, the hint and,
on a validation failure, one line per failing field. Read those
before trying again; the same call with the same body fails the
same way. A product needs a brand and at least one component;
`product_property_types` shows the property types it may carry.

## First call

```
transpareo me
```

It answers the consumer's name, its permissions, the scope of the
token and the rate limit. A permission the consumer lacks makes
every related call answer 403 with the key it would need; do not
retry such a call, report the key.

## The five core flows

1. What a passport of a product needs:
   `transpareo dpps requirements --product-id <id> --granularity serial`
   answers the identifiers the granularity needs, the inherited
   properties, what a publish would say today, and a ready-to-fill
   body under `template`.
2. Check before writing:
   `transpareo dpps validate --file passport.json` runs the checks
   a publish runs and writes nothing. It answers `valid` and
   `fields`; fix the fields until `valid` is true.
3. Write it: `transpareo dpps create --file passport.json`.
4. Publish it: `transpareo dpps publish <code>`. Publishing signs a
   snapshot into the ten-year archive and cannot be undone.
5. Follow what happened: `transpareo events list --since <cursor>`
   answers every passport event since the cursor and the next
   cursor to keep.

For a product that does not exist yet, start with
`transpareo products new`, which lists the property types the
workspace defines and which are mandatory, then
`transpareo products create --file product.json`. A product missing
a mandatory property is refused, and the answer names the missing
types under `fields.properties.missing`.

For many passports at once, `transpareo dpps bulk validate --file
rows.ndjson` and `transpareo dpps bulk create --file rows.ndjson`
take one passport per line. Rows are deduplicated by their
identifiers, so resubmitting after a timeout creates nothing
twice. Send `--prefer respond-async --wait` beyond 500 rows.

## What cannot be undone

Publishing a passport, voiding it, superseding it, deleting a
record, regenerating a webhook secret and reverting an import are
irreversible. The command line needs `--yes` on every such
command; the MCP tools need a `confirm` argument that repeats the
record's identifier, such as `void A1B2C3D4E`. Never pass either
without the user having asked for exactly that action on exactly
that record.

`--read-only` on any command, and the server started with
`--read-only`, refuse every write and keep the validations. Run
under it when the task is to inspect.

## Reading an error

Every failure has the same shape:

```
{"ok": false, "error": {"code": "DPP_INVALID", "message": "...",
  "hint": "...", "docsUrl": "...", "fields": {...}, "retryable": false}}
```

Read `hint` first: it names the permission to ask for, the URL to
poll, or the option that overrides the refusal. `fields` carries
one entry per failing attribute on a validation. `retryable` is
true for rate limits and outages only; wait and retry those, fix
and resubmit everything else. Exit codes: 1 an API or transport
error, 2 wrong usage, 3 validation failed, 4 an irreversible
command refused for lack of `--yes`, 5 a column mapping required.

## Handing a subagent a token

A subagent should never see the secret. Mint a token narrowed to
what it needs and pass it in the environment:

```
export TRANSPAREO_TOKEN=$(transpareo auth token --scope dpp_read)
export TRANSPAREO_HOST=acme.example.com
```

The token lives an hour. For a partner that must write events to
one passport, `transpareo auth grant --code <passport code>` issues
credentials confined to that passport for a day.

## Data protection

Most of what a passport holds is public by design. The private
properties (`dpps private-properties`, permission `dpp_vault_read`)
are a restricted tier; do not read them unless the task needs
them, and do not copy them into notes or messages. Product and
supplier data of the workspace is the customer's own; run the
assistant under business terms with EU processing and no retention
of conversations, and keep what it reads out of prompts that leave
the customer's control.

## Reference

Every command, generated from the API specification. `--help` on
any of them shows an example and the permission it needs;
`transpareo commands --json` prints the same for programs.

<!-- reference:start -->
- `transpareo api <METHOD> <path>`: Send an authenticated request to any endpoint
- `transpareo auth grant --code <passport code>`: Issue a child credential scoped to one passport (POST /grant) (dpp_events or dpp_history or dpp_dynamic)
- `transpareo auth login --host <workspace host> --client-id <key>`: Store a credential after checking it at the token endpoint
- `transpareo auth logout`: Remove the stored profile and its secret
- `transpareo auth status`: Show what the current credential allows (GET /me)
- `transpareo auth token [--scope a,b]`: Print a fresh short-lived bearer token for scripts and subagents
- `transpareo brands create`: Create a brand (brand_access or brand_write)
- `transpareo brands delete <id>`: Delete a brand (brand_access or brand_write)
- `transpareo brands get <id>`: Get a brand
- `transpareo brands list`: List brands
- `transpareo brands update <id>`: Rename a brand (brand_access or brand_write)
- `transpareo categories list`: List product categories
- `transpareo commands`: List every command and every API operation, for agents
- `transpareo components create`: Create a component (component_access or component_write)
- `transpareo components delete <id>`: Delete a component (component_access or component_write)
- `transpareo components get <id>`: Get a component
- `transpareo components list`: List components
- `transpareo components publish <id>`: Publish a component (component_access or component_write)
- `transpareo components unpublish <id>`: Unpublish a component (component_access or component_write)
- `transpareo components update <id>`: Update a component (component_access or component_write)
- `transpareo configuration config`: Get tenant configuration
- `transpareo configuration languages list`: List available languages
- `transpareo configuration localization`: Get localization strings
- `transpareo configuration navigation`: Get navigation items
- `transpareo coupons lookup`: Look up a coupon by code
- `transpareo doctor`: Check the setup: profile, host, token endpoint, credential, keyring
- `transpareo dpps bulk create`: Create DPPs in bulk (dpp_bulk_write)
- `transpareo dpps bulk validate`: Validate DPPs in bulk (dpp_bulk_write)
- `transpareo dpps bulk-task <taskId>`: Poll an asynchronous bulk create (dpp_bulk_write)
- `transpareo dpps correct <id>`: Correct a DPP from its source (dpp_write)
- `transpareo dpps create`: Create a DPP (dpp_write)
- `transpareo dpps delete <id>`: Delete a DPP (dpp_write)
- `transpareo dpps dynamic-data update <id>`: Update dynamic data (dpp_dynamic)
- `transpareo dpps events append <id>`: Append an event to a DPP (dpp_events)
- `transpareo dpps events list <id>`: List the event log of a DPP (dpp_history)
- `transpareo dpps get <id>`: Read a DPP (dpp_read)
- `transpareo dpps list`: List Digital Product Passports (dpp_read)
- `transpareo dpps private-properties <code>`: Read the private properties of several versions (dpp_read)
- `transpareo dpps publish <code>`: Publish a DPP (dpp_write)
- `transpareo dpps reissue <id>`: Reissue a DPP (dpp_lifecycle)
- `transpareo dpps requirements`: What a passport of a product needs (dpp_read)
- `transpareo dpps stats <id>`: Get DPP scan statistics (dpp_read)
- `transpareo dpps supersede <id>`: Supersede a DPP (dpp_lifecycle)
- `transpareo dpps update <id>`: Update a DPP (dpp_write)
- `transpareo dpps validate`: Validate a DPP payload (dpp_write)
- `transpareo dpps version-private-properties <code> <version>`: Read the private properties of one version (dpp_read)
- `transpareo dpps versions list <id>`: List the registered versions of a DPP (dpp_read)
- `transpareo dpps void <id>`: Void a DPP (dpp_lifecycle)
- `transpareo events list`: Poll the feed of passport events (dpp_history)
- `transpareo events tail [--since <cursor>] [--follow]`: Read the workspace's passport events, optionally as a live stream (dpp_history)
- `transpareo exports create [--format jsonld|csv|xlsx|sql] [--wait] [--download <path>]`: Start a passport export, wait for it and download the archive (export_access)
- `transpareo exports download <id>`: Download an export archive (export_access)
- `transpareo exports get <id>`: Poll an export (export_access)
- `transpareo grants create`: Issue a passport-scoped partner grant (dpp_events or dpp_history or dpp_dynamic)
- `transpareo guide`: Print the workspace's API guide as Markdown
- `transpareo imports create`: Upload a spreadsheet to import (import_access)
- `transpareo imports example`: Download the example spreadsheet (import_access)
- `transpareo imports execute <id>`: Execute an import (import_access)
- `transpareo imports get <id>`: Poll an import (import_access)
- `transpareo imports map <id>`: Send the column mapping of a fresh import (import_access)
- `transpareo imports revert <id>`: Revert an import (import_access)
- `transpareo imports run --file <path> --type <components|products|dpps>`: Upload, map, validate and, with --execute, run an import (import_access)
- `transpareo imports supplier-form`: Download the blank supplier form (import_access)
- `transpareo imports validate <id>`: Validate an import (import_access)
- `transpareo lots get <id>`: Get a lot (dpp_read)
- `transpareo lots list`: List lots (dpp_read)
- `transpareo mcp [--tools <group,...>] [--read-only]`: Start the Model Context Protocol server over standard input and output
- `transpareo me`: Show what the current credential allows (GET /me)
- `transpareo mediafiles create`: Upload a mediafile
- `transpareo mediafiles delete <id>`: Delete a mediafile
- `transpareo mediafiles list`: List mediafiles
- `transpareo permalinks resolve <path>`: Resolve a permalink
- `transpareo plans get <id>`: Get a subscription plan
- `transpareo plans list`: List subscription plans
- `transpareo products create`: Create a product (product_access)
- `transpareo products delete <id>`: Delete a product (product_access)
- `transpareo products featured list`: List featured products
- `transpareo products get <id>`: Get a product
- `transpareo products list`: List products
- `transpareo products mediafiles update <id>`: Update product mediafile assignments (product_access)
- `transpareo products new`: Get new product template
- `transpareo products properties list`: List product properties
- `transpareo products publish <id>`: Publish a product (product_access)
- `transpareo products unpublish <id>`: Unpublish a product (product_access)
- `transpareo products update <id>`: Update a product (product_access)
- `transpareo reference-data component-functions list`: List component functions
- `transpareo reference-data component-names list`: Autocomplete component names
- `transpareo reference-data component-properties list`: List component property categories
- `transpareo reference-data component-types list`: List component types
- `transpareo reference-data countries list`: List countries
- `transpareo reference-data product-gtins list`: Search products by GTIN
- `transpareo schema <operationId>`: Print the request schema and example of an operation
- `transpareo search catalogue`: Search products and components
- `transpareo setup <assistant>`: Install the skill and register the MCP server for claude or codex
- `transpareo tasks wait <statusUrl>`: Poll a status URL until the work is done
- `transpareo upgrade [--version <x.y.z>] [--check]`: Replace this binary with a verified release from GitHub
- `transpareo version`: Print the version of the binary and of its API specification
- `transpareo webhooks create`: Create a webhook subscription (webhook_access)
- `transpareo webhooks delete <id>`: Delete a webhook subscription (webhook_access)
- `transpareo webhooks get <id>`: Get a webhook subscription (webhook_access)
- `transpareo webhooks list`: List webhook subscriptions (webhook_access)
- `transpareo webhooks secret regenerate <id>`: Rotate a webhook secret (webhook_access)
- `transpareo webhooks test <id>`: Send a test delivery (webhook_access)
- `transpareo webhooks update <id>`: Update a webhook subscription (webhook_access)
<!-- reference:end -->
