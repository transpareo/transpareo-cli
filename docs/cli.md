# Command reference

Every command of `transpareo`, generated from API specification 2.1.0. `--help` on any command prints the same example and permission key.

| Option on every command | What it does |
|---|---|
| `--json` | print JSON; the default when the output is not a terminal |
| `--jsonl` | print lists as one JSON object per line |
| `-q`, `--quiet` | print ids only, one per line |
| `--fields a,b` | keep only these fields of the answer |
| `--profile <name>` | the workspace to use, as stored by `auth login` |
| `--read-only` | refuse every operation that changes data |
| `--yes` | confirm an operation that cannot be undone |

[General](#general) · [Brands](#brands) · [Categories](#categories) · [Components](#components) · [Configuration](#configuration) · [Coupons](#coupons) · [DPPs](#dpps) · [Events](#events) · [Exports](#exports) · [Grants](#grants) · [Imports](#imports) · [Lots](#lots) · [Mediafiles](#mediafiles) · [Permalinks](#permalinks) · [Plans](#plans) · [Products](#products) · [Reference Data](#reference-data) · [Search](#search) · [Webhooks](#webhooks)

## General

Logging in, reaching any endpoint, discovering the surface, the assistant setup and the binary itself.

### transpareo api

Send an authenticated request to any endpoint.

```sh
transpareo api <METHOD> <path>
transpareo api GET /dpps --query page=2 --query per_page=50
transpareo api POST /dpps/validate --body @passport.json
transpareo api PUT /products/42 --body '{"product": {"name": "New name"}}'
transpareo api DELETE /webhooks/7 --yes
```

| Option | What it does |
|---|---|
| `--body` \<value\> | request body: JSON, @\<file\>, or - for stdin |
| `--header` \<value\>... | extra header as Name: value (repeatable) |
| `--idempotency-key` \<value\> | Idempotency-Key for a POST (default: random) |
| `--query` \<value\>... | query parameter as name=value (repeatable) |

### transpareo auth grant

Issue a child credential scoped to one passport (POST /grant).

```sh
transpareo auth grant --code <passport code>
transpareo auth grant --code A1B2C3D4E
transpareo auth grant --gtin 04012345678901 --serial 000412
```

| Option | What it does |
|---|---|
| `--batch` \<value\> | batch identifier, with --gtin |
| `--code` \<value\> | the passport code printed on the QR code |
| `--gtin` \<value\> | GTIN of the product, for a lookup by GS1 identifiers |
| `--serial` \<value\> | serial identifier, with --gtin |

`POST /grant` · operation `create_grant` · permission `dpp_events` or `dpp_history` or `dpp_dynamic`

### transpareo auth login

Store a credential after checking it at the token endpoint.

```sh
transpareo auth login --host <workspace host> --client-id <key>
echo "$SECRET" | transpareo auth login \
    --host acme.example.com --client-id 3f6a...
transpareo auth login --host acme.example.com --client-id 3f6a... \
    --scope dpp_read,dpp_write --name acme-read
```

| Option | What it does |
|---|---|
| `--client-id` \<value\> | the consumer's key |
| `--default` | make this the default profile |
| `--host` \<value\> | workspace host, such as acme.example.com |
| `--name` \<value\> | profile name (default: the host) |
| `--scope` \<value\> | permission keys to narrow tokens to, comma separated |

`POST /oauth/token` · operation `exchange_token` · no permission needed, the endpoint is public

### transpareo auth logout

Remove the stored profile and its secret.

```sh
transpareo auth logout
transpareo auth logout --profile acme
```

### transpareo auth status

Show what the current credential allows (GET /me).

```sh
transpareo auth status
transpareo auth status --profile acme
```

`GET /me` · operation `get_me` · any consumer token

### transpareo auth token

Print a fresh short-lived bearer token for scripts and subagents.

```sh
transpareo auth token [--scope a,b]
export TRANSPAREO_TOKEN=$(transpareo auth token --scope dpp_read)
```

| Option | What it does |
|---|---|
| `--scope` \<value\> | permission keys to narrow the token to, comma separated |

`POST /oauth/token` · operation `exchange_token` · no permission needed, the endpoint is public

### transpareo commands

List every command and every API operation, for agents.

```sh
transpareo commands --json | jq '.operations[] | .operationId'
```

### transpareo doctor

Check the setup: profile, host, token endpoint, credential, keyring.

```sh
transpareo doctor
transpareo doctor --json
```

### transpareo guide

Print the workspace's API guide as Markdown.

```sh
transpareo guide | less
```

### transpareo mcp

Start the Model Context Protocol server over standard input and output.

```sh
transpareo mcp [--tools <group,...>] [--read-only]
transpareo mcp --profile acme
transpareo mcp --profile acme --tools dpps,products --read-only
```

| Option | What it does |
|---|---|
| `--tools` \<value\>... | tool groups to serve, comma separated (default: all) |

### transpareo me

Show what the current credential allows (GET /me).

```sh
transpareo me
transpareo me --json
```

`GET /me` · operation `get_me` · any consumer token

### transpareo schema

Print the request schema and example of an operation.

```sh
transpareo schema <operationId>
transpareo schema create_dpp
transpareo schema list_dpps --fields queryParams
```

### transpareo setup

Install the skill and register the MCP server for claude or codex.

```sh
transpareo setup <assistant>
transpareo setup claude --profile acme
transpareo setup codex
```

| Option | What it does |
|---|---|
| `--no-mcp` | install the skill only |
| `--no-skill` | register the MCP server only |

### transpareo signer keygen

Write the P-256 and Ed25519 keys and print their public halves.

```sh
transpareo signer keygen [--dir <path>] [--force]
transpareo signer keygen
transpareo signer keygen --dir /etc/transpareo/signer --json
```

| Option | What it does |
|---|---|
| `--dir` \<value\> | directory for the key files (default: signer under the config dir) |
| `--force` | replace existing key files |

### transpareo signer serve

Serve the signing endpoint.

```sh
transpareo signer serve --platform-key <pem | url> [--listen <addr>] [--dir <path>] [--p256-key <pem>] [--ed25519-key <pem>]
transpareo signer serve --platform-key platform.pem
transpareo signer serve --dir /etc/transpareo/signer --platform-key https://acme.example.com/.well-known/transpareo-signing-key.pem --listen :8443 --tls-cert cert.pem --tls-key key.pem
```

| Option | What it does |
|---|---|
| `--allow-unsigned` | accept requests without the platform signature (development only) |
| `--dir` \<value\> | directory of the key files [TRANSPAREO_SIGNER_DIR] (default: signer under the config dir) |
| `--ed25519-key` \<value\> | Ed25519 private key PEM (default: ed25519.pem under the signer dir) |
| `--host` \<value\> | host name of the registered endpoint URL, when the proxy rewrites the Host header [TRANSPAREO_SIGNER_HOST] |
| `--listen` \<value\> | address to listen on [TRANSPAREO_SIGNER_LISTEN] |
| `--p256-key` \<value\> | P-256 private key PEM (default: p256.pem under the signer dir) |
| `--path` \<value\> | the one route served |
| `--platform-key` \<value\> | the platform's request-signing public key: a PEM file or a URL [TRANSPAREO_PLATFORM_KEY] |
| `--tls-cert` \<value\> | TLS certificate PEM |
| `--tls-key` \<value\> | TLS private key PEM |

### transpareo tasks wait

Poll a status URL until the work is done.

```sh
transpareo tasks wait <statusUrl>
transpareo tasks wait https://acme.example.com/api/exports/42
transpareo tasks wait /dpps/bulk/507f1f77bcf86cd799439099
```

### transpareo upgrade

Replace this binary with a verified release from GitHub.

```sh
transpareo upgrade [--version <x.y.z>] [--check]
transpareo upgrade
transpareo upgrade --check
transpareo upgrade --version 1.2.0
transpareo upgrade --verify
```

| Option | What it does |
|---|---|
| `--check` | report the latest version without installing |
| `--verify` | verify this binary against its release |
| `--version` \<value\> | install this version instead of the latest |

### transpareo version

Print the version of the binary and of its API specification.

```sh
transpareo version
transpareo version --json
```

## Brands

Product brands.

### transpareo brands create

Create a brand.

```sh
transpareo brands create --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `--idempotency-key` \<value\> | makes the call safe to repeat: the same key within a day answers the result of the first call (default: random) |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

`POST /brands` · operation `create_brand` · permission `brand_access` or `brand_write`

### transpareo brands delete

Delete a brand.

```sh
transpareo brands delete <id>
transpareo brands delete <id> --yes
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

`DELETE /brands/{id}` · operation `delete_brand` · permission `brand_access` or `brand_write` · cannot be undone, needs `--yes`

### transpareo brands get

Get a brand.

```sh
transpareo brands get <id>
transpareo brands get <id>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

`GET /brands/{id}` · operation `get_brand` · no permission needed, the endpoint is public

### transpareo brands list

List brands.

```sh
transpareo brands list --page <page>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--page` \<n\> | Page number |
| `--per-page` \<n\> | Records per page (default: 100, max: 500) |
| `--published` | For a credential, 'true' lists the published records and 'false' the drafts; left out, the list carries both. A storefront reader has no drafts to ask for, so the parameter narrows nothing for one. A value that is neither answers 422 'PUBLISHED_FILTER_INVALID'. |

`GET /brands` · operation `list_brands` · no permission needed, the endpoint is public

### transpareo brands update

Rename a brand.

```sh
transpareo brands update <id>
transpareo brands update <id> --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

`PUT /brands/{id}` · operation `update_brand` · permission `brand_access` or `brand_write`

## Categories

Hierarchical product categories.

### transpareo categories list

List product categories.

```sh
transpareo categories list
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

`GET /product_categories` · operation `list_product_categories` · no permission needed, the endpoint is public

## Components

Product components: the individual parts, ingredients, or materials that make up a product.

### transpareo components create

Create a component.

```sh
transpareo components create --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `--idempotency-key` \<value\> | makes the call safe to repeat: the same key within a day answers the result of the first call (default: random) |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

`POST /components` · operation `create_component` · permission `component_access` or `component_write`

### transpareo components delete

Delete a component.

```sh
transpareo components delete <id>
transpareo components delete <id> --yes
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

`DELETE /components/{id}` · operation `delete_component` · permission `component_access` or `component_write` · cannot be undone, needs `--yes`

### transpareo components get

Get a component.

```sh
transpareo components get <id>
transpareo components get <id>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

`GET /components/{id}` · operation `get_component` · no permission needed, the endpoint is public

### transpareo components list

List components.

```sh
transpareo components list --page <page>
```

| Option | What it does |
|---|---|
| `--function-ids` \<value\> | Filter by function IDs (comma-separated) |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--page` \<n\> | Page number |
| `--per-page` \<n\> | Records per page (default: 100, max: 500) |
| `--property-category-ids` \<value\> | Filter by property category IDs (comma-separated) |
| `--published` | For a credential, 'true' lists the published records and 'false' the drafts; left out, the list carries both. A storefront reader has no drafts to ask for, so the parameter narrows nothing for one. A value that is neither answers 422 'PUBLISHED_FILTER_INVALID'. |
| `--term` \<value\> | Full-text search query |
| `--type-ids` \<value\> | Filter by component type IDs (comma-separated) |

`GET /components` · operation `list_components` · no permission needed, the endpoint is public

### transpareo components publish

Publish a component.

```sh
transpareo components publish <id>
transpareo components publish <id>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

`PUT /components/{id}/publish` · operation `publish_component` · permission `component_access` or `component_write`

### transpareo components unpublish

Unpublish a component.

```sh
transpareo components unpublish <id>
transpareo components unpublish <id>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

`PUT /components/{id}/unpublish` · operation `unpublish_component` · permission `component_access` or `component_write`

### transpareo components update

Update a component.

```sh
transpareo components update <id>
transpareo components update <id> --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

`PUT /components/{id}` · operation `update_component` · permission `component_access` or `component_write`

## Configuration

Tenant configuration, localization, navigation, and languages.

### transpareo configuration config

Get tenant configuration.

```sh
transpareo configuration config
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

`GET /config` · operation `get_config` · no permission needed, the endpoint is public

### transpareo configuration languages list

List available languages.

```sh
transpareo configuration languages list
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

`GET /languages` · operation `list_languages` · no permission needed, the endpoint is public

### transpareo configuration localization

Get localization strings.

```sh
transpareo configuration localization
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

`GET /localization` · operation `get_localization` · no permission needed, the endpoint is public

### transpareo configuration navigation

Get navigation items.

```sh
transpareo configuration navigation
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

`GET /navigation` · operation `get_navigation` · no permission needed, the endpoint is public

## Coupons

Discount coupon lookup.

### transpareo coupons lookup

Look up a coupon by code.

```sh
transpareo coupons lookup --code <code>
```

| Option | What it does |
|---|---|
| `--code` \<value\> | Coupon code (case-insensitive) |
| `--currency` \<value\> | Currency code to filter by (e.g. EUR, USD) |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

`GET /coupons` · operation `lookup_coupon` · no permission needed, the endpoint is public

## DPPs

Digital Product Passports: QR code links to product data, compliant with EU ESPR regulation.

### transpareo dpps bulk create

Create DPPs in bulk.

```sh
transpareo dpps bulk create --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--prefer` \<value\> | Send 'respond-async' to process the batch in the background and answer 202 with a polling URL. |
| `--wait` | poll the statusUrl until the work is done |

`POST /dpps/bulk` · operation `bulk_create_dpps` · permission `dpp_bulk_write`

### transpareo dpps bulk validate

Validate DPPs in bulk.

```sh
transpareo dpps bulk validate --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

`POST /dpps/bulk/validate` · operation `validate_dpps_bulk` · permission `dpp_bulk_write`

### transpareo dpps bulk-task

Poll an asynchronous bulk create.

```sh
transpareo dpps bulk-task <taskId>
transpareo dpps bulk-task <taskId>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

`GET /dpps/bulk/{taskId}` · operation `get_bulk_task` · permission `dpp_bulk_write`

### transpareo dpps correct

Correct a DPP from its source.

```sh
transpareo dpps correct <id>
transpareo dpps correct <id> --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `--idempotency-key` \<value\> | makes the call safe to repeat: the same key within a day answers the result of the first call (default: random) |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

`POST /dpps/{id}/correct` · operation `correct_dpp` · permission `dpp_write`

### transpareo dpps create

Create a DPP.

```sh
transpareo dpps create --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `--idempotency-key` \<value\> | makes the call safe to repeat: the same key within a day answers the result of the first call (default: random) |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

`POST /dpps` · operation `create_dpp` · permission `dpp_write`

### transpareo dpps delete

Delete a DPP.

```sh
transpareo dpps delete <id>
transpareo dpps delete <id> --yes
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

`DELETE /dpps/{id}` · operation `delete_dpp` · permission `dpp_write` · cannot be undone, needs `--yes`

### transpareo dpps dynamic-data update

Update dynamic data.

```sh
transpareo dpps dynamic-data update <id>
transpareo dpps dynamic-data update <id> --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

`PATCH /dpps/{id}/dynamic_data` · operation `update_dpp_dynamic_data` · permission `dpp_dynamic`

### transpareo dpps events append

Append an event to a DPP.

```sh
transpareo dpps events append <id>
transpareo dpps events append <id> --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

`POST /dpps/{id}/events` · operation `append_dpp_event` · permission `dpp_events`

### transpareo dpps events list

List the event log of a DPP.

```sh
transpareo dpps events list <id>
transpareo dpps events list <id> --version <version>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--version` \<n\> | Return only the events sealed into this version |

`GET /dpps/{id}/events` · operation `list_dpp_events` · permission `dpp_history`

### transpareo dpps get

Read a DPP.

```sh
transpareo dpps get <id>
transpareo dpps get <id> --version <version>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--version` \<n\> | Return the historical signed snapshot for this version number instead of the passport as it stands. |

`GET /dpps/{id}` · operation `get_dpp` · permission `dpp_read`

### transpareo dpps list

List Digital Product Passports.

```sh
transpareo dpps list --term <term>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--page` \<n\> | Page number |
| `--per-page` \<n\> | Records per page (default: 100, max: 500) |
| `--product-id` \<n\> | Keep only the passports of this product |
| `--status` \<value\> | Keep only the passports in these lifecycle statuses, comma-separated: draft, manufactured, placed_on_market, in_use, repair, refurbished, collected, recycled, end_of_life, suspended |
| `--term` \<value\> | Search DPPs by code or description |

`GET /dpps` · operation `list_dpps` · permission `dpp_read`

### transpareo dpps private-properties

Read the private properties of several versions.

```sh
transpareo dpps private-properties <code>
transpareo dpps private-properties <code> --versions <versions>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--versions` \<value\> | Comma-separated version numbers ('versions[]=' is also accepted). At most 50 are derived. |

`GET /dpps/{code}/private_properties` · operation `get_dpp_private_properties` · permission `dpp_read`

### transpareo dpps publish

Publish a DPP.

```sh
transpareo dpps publish <code>
transpareo dpps publish <code> --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `--idempotency-key` \<value\> | makes the call safe to repeat: the same key within a day answers the result of the first call (default: random) |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

`POST /dpps/{code}/publish` · operation `publish_dpp` · permission `dpp_write`

### transpareo dpps reissue

Reissue a DPP.

```sh
transpareo dpps reissue <id>
transpareo dpps reissue <id> --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `--idempotency-key` \<value\> | makes the call safe to repeat: the same key within a day answers the result of the first call (default: random) |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

`POST /dpps/{id}/reissue` · operation `reissue_dpp` · permission `dpp_lifecycle`

### transpareo dpps requirements

What a passport of a product needs.

```sh
transpareo dpps requirements --product-id <productId>
```

| Option | What it does |
|---|---|
| `--granularity` \<value\> | Which unit the passport would stand for. Defaults to the tenant's setting, which the answer repeats as 'defaultGranularity'. |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--product-id` \<n\> | The product a passport would describe. The snake_case spelling 'product_id' is accepted as well. |

`GET /dpps/requirements` · operation `get_dpp_requirements` · permission `dpp_read`

### transpareo dpps stats

Get DPP scan statistics.

```sh
transpareo dpps stats <id>
transpareo dpps stats <id> --format <format>
```

| Option | What it does |
|---|---|
| `--format` \<value\> | Response format |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

`GET /dpps/{id}/stats` · operation `get_dpp_stats` · permission `dpp_read`

### transpareo dpps supersede

Supersede a DPP.

```sh
transpareo dpps supersede <id>
transpareo dpps supersede <id> --file body.json --yes
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `--idempotency-key` \<value\> | makes the call safe to repeat: the same key within a day answers the result of the first call (default: random) |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

`POST /dpps/{id}/supersede` · operation `supersede_dpp` · permission `dpp_lifecycle` · cannot be undone, needs `--yes`

### transpareo dpps update

Update a DPP.

```sh
transpareo dpps update <id>
transpareo dpps update <id> --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

`PUT /dpps/{id}` · operation `update_dpp` · permission `dpp_write`

### transpareo dpps validate

Validate a DPP payload.

```sh
transpareo dpps validate --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

`POST /dpps/validate` · operation `validate_dpp` · permission `dpp_write`

### transpareo dpps version-private-properties

Read the private properties of one version.

```sh
transpareo dpps version-private-properties <code> <version>
transpareo dpps version-private-properties <code> <version>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

`GET /dpps/{code}/private_properties/{version}` · operation `get_dpp_version_private_properties` · permission `dpp_read`

### transpareo dpps versions list

List the registered versions of a DPP.

```sh
transpareo dpps versions list <id>
transpareo dpps versions list <id>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

`GET /dpps/{id}/versions` · operation `list_dpp_versions` · permission `dpp_read`

### transpareo dpps void

Void a DPP.

```sh
transpareo dpps void <id>
transpareo dpps void <id> --file body.json --yes
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `--idempotency-key` \<value\> | makes the call safe to repeat: the same key within a day answers the result of the first call (default: random) |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

`POST /dpps/{id}/void` · operation `void_dpp` · permission `dpp_lifecycle` · cannot be undone, needs `--yes`

## Events

The workspace-wide feed of passport events, polled by cursor.

### transpareo events list

Poll the feed of passport events.

```sh
transpareo events list --since <since>
```

| Option | What it does |
|---|---|
| `--dpp-code` \<value\> | Confine the feed to the passport with this public code |
| `--limit` \<n\> | Events per answer |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--since` \<value\> | The 'nextCursor' of the previous answer, or an ISO 8601 time to start from |
| `--types` \<value\> | Comma-separated event types to keep, out of the 'eventType' values of 'DppEvent' |

`GET /events` · operation `list_events` · permission `dpp_history`

### transpareo events tail

Read the workspace's passport events, optionally as a live stream.

```sh
transpareo events tail [--since <cursor>] [--follow]
transpareo events tail --since 2026-09-01T00:00:00Z
transpareo events tail --follow --types published,voided
```

| Option | What it does |
|---|---|
| `--dpp-code` \<value\> | confine the feed to one passport |
| `--follow` | keep polling and print new events as they arrive |
| `--interval` \<duration\> | wait between polls with --follow |
| `--limit` \<n\> | events per answer (default 100, max 500) |
| `--since` \<value\> | cursor of the previous answer, or an ISO 8601 time |
| `--types` \<value\> | event types to keep, comma separated |

`GET /events` · operation `list_events` · permission `dpp_history`

## Exports

Passport exports: started by a consumer, polled, and downloaded as an archive.

### transpareo exports create

Start a passport export, wait for it and download the archive.

```sh
transpareo exports create [--format jsonld|csv|xlsx|sql] [--wait] [--download <path>]
transpareo exports create --format jsonld --wait \
    --download catalogue.tar.gz
transpareo exports create --format csv --normalize
```

| Option | What it does |
|---|---|
| `--download` \<value\> | write the finished archive to this path |
| `--format` \<value\> | jsonld (default), csv, xlsx or sql |
| `--include-media` | copy media files into the archive |
| `--normalize` | resolve references into the rows |
| `--wait` | poll until the export is done |

`POST /exports` · operation `create_export` · permission `export_access`

### transpareo exports download

Download an export archive.

```sh
transpareo exports download <id>
transpareo exports download <id>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

`GET /exports/{id}/download` · operation `download_export` · permission `export_access`

### transpareo exports get

Poll an export.

```sh
transpareo exports get <id>
transpareo exports get <id>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--wait` | poll the statusUrl until the work is done |

`GET /exports/{id}` · operation `get_export` · permission `export_access`

## Grants

Self-service partner grants: short-lived credentials scoped to one passport, issued from a scanned code.

### transpareo grants create

Issue a passport-scoped partner grant.

```sh
transpareo grants create --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `--idempotency-key` \<value\> | makes the call safe to repeat: the same key within a day answers the result of the first call (default: random) |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

`POST /grant` · operation `create_grant` · permission `dpp_events` or `dpp_history` or `dpp_dynamic`

## Imports

Spreadsheet imports: upload, map the columns, validate, execute, revert.

### transpareo imports create

Upload a spreadsheet to import.

```sh
transpareo imports create --file <path>
```

| Option | What it does |
|---|---|
| `--data-type` \<value\> |  |
| `--file` \<value\> | path of the file to upload; The spreadsheet, at most 50 MB, 100000 rows and 2000000 cells |
| `--mappings` \<value\> | The mapping of 'ImportMappingsInput', sent as nested form fields (JSON) |
| `--options` \<value\> | The options of 'ImportMappingsInput', sent as nested form fields (JSON) |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--value-separator` \<value\> | What separates several values in one cell |
| `--wait` | poll the statusUrl until the work is done |

`POST /imports` · operation `create_import` · permission `import_access`

### transpareo imports example

Download the example spreadsheet.

```sh
transpareo imports example --data-type <dataType>
```

| Option | What it does |
|---|---|
| `--data-type` \<value\> |  |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

`GET /imports/example` · operation `get_import_example` · permission `import_access`

### transpareo imports execute

Execute an import.

```sh
transpareo imports execute <id>
transpareo imports execute <id> --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |
| `--wait` | poll the statusUrl until the work is done |

`POST /imports/{id}/execute` · operation `execute_import` · permission `import_access`

### transpareo imports get

Poll an import.

```sh
transpareo imports get <id>
transpareo imports get <id>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--wait` | poll the statusUrl until the work is done |

`GET /imports/{id}` · operation `get_import` · permission `import_access`

### transpareo imports map

Send the column mapping of a fresh import.

```sh
transpareo imports map <id>
transpareo imports map 12 --mappings mapping.json
transpareo imports map 12 --map "Artikelname=name" \
    --map "Gewicht=property:Weight" --map "Farbe=new:Colour" \
    --map "Intern=skip"
transpareo imports map 12 --accept-suggestions
```

| Option | What it does |
|---|---|
| `--accept-suggestions` | take every exact match of the preview |
| `--map` \<value\>... | Column=target (repeatable) |
| `--mappings` \<value\> | mapping file (ImportMappingsInput) |
| `--published` | publish the records the import creates |
| `--skip-backup` | execute without the backup a revert needs |

`PUT /imports/{id}/mappings` · operation `map_import` · permission `import_access`

### transpareo imports revert

Revert an import.

```sh
transpareo imports revert <id>
transpareo imports revert <id> --file body.json --yes
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |
| `--wait` | poll the statusUrl until the work is done |

`POST /imports/{id}/revert` · operation `revert_import` · permission `import_access` · cannot be undone, needs `--yes`

### transpareo imports run

Upload, map, validate and, with --execute, run an import.

```sh
transpareo imports run --file <path> --type <components|products|dpps>
transpareo imports run --file catalogue.xlsx --type products
transpareo imports run --file catalogue.xlsx --type products \
    --accept-suggestions --map "Farbe=new:Colour" --execute
transpareo imports run --file catalogue.xlsx --type products --auto
```

| Option | What it does |
|---|---|
| `--accept-suggestions` | take every exact match of the preview |
| `--auto` | let the platform map and write in one call, making a property type for every column that matches none |
| `--execute` | write the records when the validation passes |
| `--file` \<value\> | the spreadsheet or JSON file to import |
| `--map` \<value\>... | Column=target (repeatable) |
| `--mappings` \<value\> | mapping file (ImportMappingsInput) |
| `--published` | publish the records the import creates |
| `--skip-backup` | execute without the backup a revert needs |
| `--type` \<value\> | components, products or dpps |
| `--value-separator` \<value\> | what separates several values in a cell |

`POST /imports/{id}/execute` · operation `execute_import` · permission `import_access`

### transpareo imports supplier-form

Download the blank supplier form.

```sh
transpareo imports supplier-form --data-type <dataType>
```

| Option | What it does |
|---|---|
| `--data-type` \<value\> |  |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--sheet-locale` \<value\> | The language of the form; a language the workspace does not run falls back to the request's |

`GET /imports/supplier_form` · operation `get_import_supplier_form` · permission `import_access`

### transpareo imports validate

Validate an import.

```sh
transpareo imports validate <id>
transpareo imports validate <id>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--wait` | poll the statusUrl until the work is done |

`POST /imports/{id}/validate` · operation `validate_import` · permission `import_access`

## Lots

The lots batch and item passports freeze from, created with the first passport that names them.

### transpareo lots get

Get a lot.

```sh
transpareo lots get <id>
transpareo lots get <id>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

`GET /lots/{id}` · operation `get_lot` · permission `dpp_read`

### transpareo lots list

List lots.

```sh
transpareo lots list --product-id <product_id>
```

| Option | What it does |
|---|---|
| `--identifier` \<value\> | Keep only the lot with this identifier, the value a passport carries as 'batchIdentifier' |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--page` \<n\> | Page number |
| `--per-page` \<n\> | Records per page (default: 100, max: 500) |
| `--product-id` \<n\> | Keep only the lots of this product |

`GET /lots` · operation `list_lots` · permission `dpp_read`

## Mediafiles

Product image uploads and management.

### transpareo mediafiles create

Upload a mediafile.

```sh
transpareo mediafiles create --file <path> --upload <path>
```

| Option | What it does |
|---|---|
| `--file` \<value\> | path of the file to upload; Image file to upload |
| `--name` \<value\> | Optional display name |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--upload` \<value\> | path of the file to upload; The same file under the name the model uses; send either this or 'mediafile[file]' |

`POST /mediafiles` · operation `create_mediafile` · any consumer token

### transpareo mediafiles delete

Delete a mediafile.

```sh
transpareo mediafiles delete <id>
transpareo mediafiles delete <id> --yes
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

`DELETE /mediafiles/{id}` · operation `delete_mediafile` · any consumer token · cannot be undone, needs `--yes`

### transpareo mediafiles list

List mediafiles.

```sh
transpareo mediafiles list --page <page>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--page` \<n\> | Page number |
| `--per-page` \<n\> | Records per page (default: 100, max: 500) |
| `--term` \<value\> | Filter by display name or stored file name |

`GET /mediafiles` · operation `list_mediafiles` · any consumer token

## Permalinks

Resolve permalink paths to products, components, or brands.

### transpareo permalinks resolve

Resolve a permalink.

```sh
transpareo permalinks resolve <path>
transpareo permalinks resolve <path> --show <show>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--show` \<value\> | Comma-separated property type IDs to reveal restricted properties |

`GET /permalinks/{path}` · operation `resolve_permalink` · no permission needed, the endpoint is public

## Plans

Subscription plans and pricing.

### transpareo plans get

Get a subscription plan.

```sh
transpareo plans get <id>
transpareo plans get <id>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

`GET /plans/{id}` · operation `get_plan` · no permission needed, the endpoint is public

### transpareo plans list

List subscription plans.

```sh
transpareo plans list
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

`GET /plans` · operation `list_plans` · no permission needed, the endpoint is public

## Products

Product data including ratings, components, and properties.

### transpareo products create

Create a product.

```sh
transpareo products create --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `--idempotency-key` \<value\> | makes the call safe to repeat: the same key within a day answers the result of the first call (default: random) |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

`POST /products` · operation `create_product` · permission `product_access`

### transpareo products delete

Delete a product.

```sh
transpareo products delete <id>
transpareo products delete <id> --yes
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

`DELETE /products/{id}` · operation `delete_product` · permission `product_access` · cannot be undone, needs `--yes`

### transpareo products featured list

List featured products.

```sh
transpareo products featured list
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

`GET /featured_products` · operation `list_featured_products` · no permission needed, the endpoint is public

### transpareo products get

Get a product.

```sh
transpareo products get <id>
transpareo products get <id> --show <show>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--raw` | Return raw property values instead of rendered HTML |
| `--show` \<value\> | Comma-separated property type IDs to reveal restricted properties |

`GET /products/{id}` · operation `get_product` · no permission needed, the endpoint is public

### transpareo products list

List products.

```sh
transpareo products list --page <page>
```

| Option | What it does |
|---|---|
| `--brand-id` \<n\> | Filter by brand ID |
| `--category-ids` \<value\> | Filter by category IDs (comma-separated). Includes all child categories. |
| `--exact` | When true with 'term', match exact product name |
| `--function-ids` \<value\> | Filter by component function IDs (comma-separated) |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--page` \<n\> | Page number |
| `--per-page` \<n\> | Records per page (default: 100, max: 500) |
| `--property-category-ids` \<value\> | Filter by component property category IDs (comma-separated) |
| `--published` | For a credential, 'true' lists the published records and 'false' the drafts; left out, the list carries both. A storefront reader has no drafts to ask for, so the parameter narrows nothing for one. A value that is neither answers 422 'PUBLISHED_FILTER_INVALID'. |
| `--rating` \<value\> | Filter by rating (comma-separated values: A, B, C, D) |
| `--term` \<value\> | Full-text search query (uses Elasticsearch when provided) |
| `--type-ids` \<value\> | Filter by component type IDs (comma-separated) |

`GET /products` · operation `list_products` · no permission needed, the endpoint is public

### transpareo products mediafiles update

Update product mediafile assignments.

```sh
transpareo products mediafiles update <id>
transpareo products mediafiles update <id> --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

`PUT /products/{id}/mediafiles` · operation `update_product_mediafiles` · permission `product_access`

### transpareo products new

Get new product template.

```sh
transpareo products new
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

`GET /products/new` · operation `get_new_product` · no permission needed, the endpoint is public

### transpareo products properties list

List product properties.

```sh
transpareo products properties list --page <page>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--page` \<n\> | Page number |
| `--per-page` \<n\> | Records per page (default: 100, max: 500) |
| `--property-type-id` \<n\> | The property type whose values to list. Without it, and for a type that does not exist, the list is empty. |
| `--term` \<value\> | Filter the values by their text |

`GET /product_properties` · operation `list_product_properties` · no permission needed, the endpoint is public

### transpareo products publish

Publish a product.

```sh
transpareo products publish <id>
transpareo products publish <id>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

`PUT /products/{id}/publish` · operation `publish_product` · permission `product_access`

### transpareo products unpublish

Unpublish a product.

```sh
transpareo products unpublish <id>
transpareo products unpublish <id>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

`PUT /products/{id}/unpublish` · operation `unpublish_product` · permission `product_access`

### transpareo products update

Update a product.

```sh
transpareo products update <id>
transpareo products update <id> --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

`PUT /products/{id}` · operation `update_product` · permission `product_access`

## Reference Data

Component names, functions, types, property categories, countries, and GTINs.

### transpareo reference-data component-functions list

List component functions.

```sh
transpareo reference-data component-functions list
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

`GET /component_functions` · operation `list_component_functions` · no permission needed, the endpoint is public

### transpareo reference-data component-names list

Autocomplete component names.

```sh
transpareo reference-data component-names list --page <page>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--page` \<n\> | Page number |
| `--per-page` \<n\> | Records per page (default: 100, max: 500) |
| `--term` \<value\> | Prefix to search for |

`GET /component_names` · operation `list_component_names` · no permission needed, the endpoint is public

### transpareo reference-data component-properties list

List component property categories.

```sh
transpareo reference-data component-properties list
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

`GET /component_properties` · operation `list_component_properties` · no permission needed, the endpoint is public

### transpareo reference-data component-types list

List component types.

```sh
transpareo reference-data component-types list
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

`GET /component_types` · operation `list_component_types` · no permission needed, the endpoint is public

### transpareo reference-data countries list

List countries.

```sh
transpareo reference-data countries list
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

`GET /countries` · operation `list_countries` · no permission needed, the endpoint is public

### transpareo reference-data product-gtins list

Search products by GTIN.

```sh
transpareo reference-data product-gtins list --page <page>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--page` \<n\> | Page number |
| `--per-page` \<n\> | Records per page (default: 100, max: 500) |
| `--term` \<value\> | GTIN prefix to search for |

`GET /product_gtins` · operation `list_product_gtins` · no permission needed, the endpoint is public

## Search

Full-text search across products and components.

### transpareo search catalogue

Search products and components.

```sh
transpareo search catalogue --query <query>
```

| Option | What it does |
|---|---|
| `--models` \<value\> | Comma-separated model types to search (e.g. 'products', 'components') |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--page` \<n\> | Page number |
| `--per-page` \<n\> | Records per page (default: 100, max: 500) |
| `--query` \<value\> | Search query |

`GET /search` · operation `search_catalogue` · no permission needed, the endpoint is public

## Webhooks

Webhook subscriptions a consumer manages for itself: signed deliveries of versioned passport events.

### transpareo webhooks create

Create a webhook subscription.

```sh
transpareo webhooks create --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `--idempotency-key` \<value\> | makes the call safe to repeat: the same key within a day answers the result of the first call (default: random) |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

`POST /webhooks` · operation `create_webhook` · permission `webhook_access`

### transpareo webhooks delete

Delete a webhook subscription.

```sh
transpareo webhooks delete <id>
transpareo webhooks delete <id> --yes
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

`DELETE /webhooks/{id}` · operation `delete_webhook` · permission `webhook_access` · cannot be undone, needs `--yes`

### transpareo webhooks get

Get a webhook subscription.

```sh
transpareo webhooks get <id>
transpareo webhooks get <id>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

`GET /webhooks/{id}` · operation `get_webhook` · permission `webhook_access`

### transpareo webhooks list

List webhook subscriptions.

```sh
transpareo webhooks list --page <page>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--page` \<n\> | Page number |
| `--per-page` \<n\> | Records per page (default: 100, max: 500) |

`GET /webhooks` · operation `list_webhooks` · permission `webhook_access`

### transpareo webhooks secret regenerate

Rotate a webhook secret.

```sh
transpareo webhooks secret regenerate <id>
transpareo webhooks secret regenerate <id> --yes
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

`POST /webhooks/{id}/regenerate_secret` · operation `regenerate_webhook_secret` · permission `webhook_access` · cannot be undone, needs `--yes`

### transpareo webhooks test

Send a test delivery.

```sh
transpareo webhooks test <id>
transpareo webhooks test <id>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

`POST /webhooks/{id}/test` · operation `test_webhook` · permission `webhook_access`

### transpareo webhooks update

Update a webhook subscription.

```sh
transpareo webhooks update <id>
transpareo webhooks update <id> --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

`PUT /webhooks/{id}` · operation `update_webhook` · permission `webhook_access`

