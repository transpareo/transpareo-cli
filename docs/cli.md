# Command reference

Generated from specification 1.6.0 by `go generate ./...`; do not edit by hand.

Every command accepts `--json`, `--jsonl`, `-q`, `--fields`, `--profile`, `--read-only` and `--yes`; the README says what they do. `--help` on any command prints the same example and permission key as this page.

- [General](#general)
- [Brands](#brands)
- [Categories](#categories)
- [Components](#components)
- [Configuration](#configuration)
- [Coupons](#coupons)
- [DPPs](#dpps)
- [Events](#events)
- [Exports](#exports)
- [Grants](#grants)
- [Imports](#imports)
- [Mediafiles](#mediafiles)
- [Permalinks](#permalinks)
- [Plans](#plans)
- [Products](#products)
- [Reference Data](#reference-data)
- [Search](#search)
- [Webhooks](#webhooks)

## General

### transpareo api

Send an authenticated request to any endpoint

```
transpareo api <METHOD> <path>
```

Example:

```
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

Issue a child credential scoped to one passport (POST /grant)

```
transpareo auth grant --code <passport code>
```

Example:

```
transpareo auth grant --code A1B2C3D4E
transpareo auth grant --gtin 04012345678901 --serial 000412
```

| Option | What it does |
|---|---|
| `--batch` \<value\> | batch identifier, with --gtin |
| `--code` \<value\> | the passport code printed on the QR code |
| `--gtin` \<value\> | GTIN of the product, for a lookup by GS1 identifiers |
| `--serial` \<value\> | serial identifier, with --gtin |

**API:** `POST /grant` (`create_grant`). **Permission:** `dpp_events` or `dpp_history` or `dpp_dynamic`.

### transpareo auth login

Store a credential after checking it at the token endpoint

```
transpareo auth login --host <workspace host> --client-id <key>
```

Example:

```
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

**API:** `POST /oauth/token` (`exchange_token`). **Permission:** none, the endpoint is public.

### transpareo auth logout

Remove the stored profile and its secret

```
transpareo auth logout
```

### transpareo auth status

Show what the current credential allows (GET /me)

```
transpareo auth status
```

**API:** `GET /me` (`get_me`). **Permission:** any consumer token.

### transpareo auth token

Print a fresh short-lived bearer token for scripts and subagents

```
transpareo auth token [--scope a,b]
```

Example:

```
export TRANSPAREO_TOKEN=$(transpareo auth token \
    --scope dpp_read)
```

| Option | What it does |
|---|---|
| `--scope` \<value\> | permission keys to narrow the token to, comma separated |

**API:** `POST /oauth/token` (`exchange_token`). **Permission:** none, the endpoint is public.

### transpareo commands

List every command and every API operation, for agents

```
transpareo commands
```

Example:

```
transpareo commands --json | jq '.operations[] | .operationId'
```

### transpareo doctor

Check the setup: profile, host, token endpoint, credential, keyring

```
transpareo doctor
```

Example:

```
transpareo doctor
transpareo doctor --json
```

### transpareo guide

Print the workspace's API guide as Markdown

```
transpareo guide
```

Example:

```
transpareo guide | less
```

### transpareo mcp

Start the Model Context Protocol server over standard input and output

```
transpareo mcp [--tools <group,...>] [--read-only]
```

Example:

```
transpareo mcp --profile acme
transpareo mcp --profile acme --tools dpps,products --read-only
```

| Option | What it does |
|---|---|
| `--tools` \<value\>... | tool groups to serve, comma separated (default: all) |

### transpareo me

Show what the current credential allows (GET /me)

```
transpareo me
```

Example:

```
transpareo me
transpareo me --json
```

**API:** `GET /me` (`get_me`). **Permission:** any consumer token.

### transpareo schema

Print the request schema and example of an operation

```
transpareo schema <operationId>
```

Example:

```
transpareo schema create_dpp
transpareo schema list_dpps --fields queryParams
```

### transpareo setup

Install the skill and register the MCP server for claude or codex

```
transpareo setup <assistant>
```

Example:

```
transpareo setup claude --profile acme
transpareo setup codex
```

| Option | What it does |
|---|---|
| `--no-mcp` | install the skill only |
| `--no-skill` | register the MCP server only |

### transpareo upgrade

Replace this binary with a verified release from GitHub

```
transpareo upgrade [--version <x.y.z>] [--check]
```

Example:

```
transpareo upgrade
transpareo upgrade --check
transpareo upgrade --version 1.2.0
```

| Option | What it does |
|---|---|
| `--check` | report the latest version without installing |
| `--version` \<value\> | install this version instead of the latest |

### transpareo version

Print the version of the binary and of its API specification

```
transpareo version
```

## Brands

### transpareo brands create

Create a brand

```
transpareo brands create
```

Example:

```
transpareo brands create --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

**API:** `POST /brands` (`create_brand`). **Permission:** `brand_access` or `brand_write`.

### transpareo brands delete

Delete a brand

```
transpareo brands delete <id>
```

Example:

```
transpareo brands delete <id> --yes
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

**API:** `DELETE /brands/{id}` (`delete_brand`). **Permission:** `brand_access` or `brand_write`. **Cannot be undone**; needs `--yes`.

### transpareo brands get

Get a brand

```
transpareo brands get <id>
```

Example:

```
transpareo brands get <id>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

**API:** `GET /brands/{id}` (`get_brand`). **Permission:** none, the endpoint is public.

### transpareo brands list

List brands

```
transpareo brands list
```

Example:

```
transpareo brands list --page <page>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--page` \<n\> | Page number |
| `--per-page` \<n\> | Records per page (default: 100, max: 500) |

**API:** `GET /brands` (`list_brands`). **Permission:** none, the endpoint is public.

### transpareo brands update

Rename a brand

```
transpareo brands update <id>
```

Example:

```
transpareo brands update <id> --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

**API:** `PUT /brands/{id}` (`update_brand`). **Permission:** `brand_access` or `brand_write`.

## Categories

### transpareo categories list

List product categories

```
transpareo categories list
```

Example:

```
transpareo categories list
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

**API:** `GET /product_categories` (`list_product_categories`). **Permission:** none, the endpoint is public.

## Components

### transpareo components create

Create a component

```
transpareo components create
```

Example:

```
transpareo components create --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

**API:** `POST /components` (`create_component`). **Permission:** `component_access` or `component_write`.

### transpareo components delete

Delete a component

```
transpareo components delete <id>
```

Example:

```
transpareo components delete <id> --yes
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

**API:** `DELETE /components/{id}` (`delete_component`). **Permission:** `component_access` or `component_write`. **Cannot be undone**; needs `--yes`.

### transpareo components get

Get a component

```
transpareo components get <id>
```

Example:

```
transpareo components get <id>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

**API:** `GET /components/{id}` (`get_component`). **Permission:** none, the endpoint is public.

### transpareo components list

List components

```
transpareo components list
```

Example:

```
transpareo components list --page <page>
```

| Option | What it does |
|---|---|
| `--function-ids` \<value\> | Filter by function IDs (comma-separated) |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--page` \<n\> | Page number |
| `--per-page` \<n\> | Records per page (default: 100, max: 500) |
| `--property-category-ids` \<value\> | Filter by property category IDs (comma-separated) |
| `--term` \<value\> | Full-text search query |
| `--type-ids` \<value\> | Filter by component type IDs (comma-separated) |

**API:** `GET /components` (`list_components`). **Permission:** none, the endpoint is public.

### transpareo components publish

Publish a component

```
transpareo components publish <id>
```

Example:

```
transpareo components publish <id>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

**API:** `PUT /components/{id}/publish` (`publish_component`). **Permission:** `component_access` or `component_write`.

### transpareo components unpublish

Unpublish a component

```
transpareo components unpublish <id>
```

Example:

```
transpareo components unpublish <id>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

**API:** `PUT /components/{id}/unpublish` (`unpublish_component`). **Permission:** `component_access` or `component_write`.

### transpareo components update

Update a component

```
transpareo components update <id>
```

Example:

```
transpareo components update <id> --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

**API:** `PUT /components/{id}` (`update_component`). **Permission:** `component_access` or `component_write`.

## Configuration

### transpareo configuration config

Get tenant configuration

```
transpareo configuration config
```

Example:

```
transpareo configuration config
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

**API:** `GET /config` (`get_config`). **Permission:** none, the endpoint is public.

### transpareo configuration languages list

List available languages

```
transpareo configuration languages list
```

Example:

```
transpareo configuration languages list
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

**API:** `GET /languages` (`list_languages`). **Permission:** none, the endpoint is public.

### transpareo configuration localization

Get localization strings

```
transpareo configuration localization
```

Example:

```
transpareo configuration localization
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

**API:** `GET /localization` (`get_localization`). **Permission:** none, the endpoint is public.

### transpareo configuration navigation

Get navigation items

```
transpareo configuration navigation
```

Example:

```
transpareo configuration navigation
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

**API:** `GET /navigation` (`get_navigation`). **Permission:** none, the endpoint is public.

## Coupons

### transpareo coupons lookup

Look up a coupon by code

```
transpareo coupons lookup
```

Example:

```
transpareo coupons lookup --code <code>
```

| Option | What it does |
|---|---|
| `--code` \<value\> | Coupon code (case-insensitive) |
| `--currency` \<value\> | Currency code to filter by (e.g. EUR, USD) |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

**API:** `GET /coupons` (`lookup_coupon`). **Permission:** none, the endpoint is public.

## DPPs

### transpareo dpps bulk create

Create DPPs in bulk

```
transpareo dpps bulk create
```

Example:

```
transpareo dpps bulk create --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--prefer` \<value\> | Send 'respond-async' to process the batch in the background and answer 202 with a polling URL. |
| `--wait` | poll the statusUrl until the work is done |

**API:** `POST /dpps/bulk` (`bulk_create_dpps`). **Permission:** `dpp_bulk_write`.

### transpareo dpps bulk validate

Validate DPPs in bulk

```
transpareo dpps bulk validate
```

Example:

```
transpareo dpps bulk validate --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

**API:** `POST /dpps/bulk/validate` (`validate_dpps_bulk`). **Permission:** `dpp_bulk_write`.

### transpareo dpps bulk-task

Poll an asynchronous bulk create

```
transpareo dpps bulk-task <taskId>
```

Example:

```
transpareo dpps bulk-task <taskId>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

**API:** `GET /dpps/bulk/{taskId}` (`get_bulk_task`). **Permission:** `dpp_bulk_write`.

### transpareo dpps create

Create a DPP

```
transpareo dpps create
```

Example:

```
transpareo dpps create --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

**API:** `POST /dpps` (`create_dpp`). **Permission:** `dpp_write`.

### transpareo dpps delete

Delete a DPP

```
transpareo dpps delete <id>
```

Example:

```
transpareo dpps delete <id> --yes
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

**API:** `DELETE /dpps/{id}` (`delete_dpp`). **Permission:** `dpp_write`. **Cannot be undone**; needs `--yes`.

### transpareo dpps dynamic-data update

Update dynamic data

```
transpareo dpps dynamic-data update <id>
```

Example:

```
transpareo dpps dynamic-data update <id> --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

**API:** `PATCH /dpps/{id}/dynamic_data` (`update_dpp_dynamic_data`). **Permission:** `dpp_dynamic`.

### transpareo dpps events append

Append an event to a DPP

```
transpareo dpps events append <id>
```

Example:

```
transpareo dpps events append <id> --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

**API:** `POST /dpps/{id}/events` (`append_dpp_event`). **Permission:** `dpp_events`.

### transpareo dpps events list

List the event log of a DPP

```
transpareo dpps events list <id>
```

Example:

```
transpareo dpps events list <id> --version <version>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--version` \<n\> | Return only the events sealed into this version |

**API:** `GET /dpps/{id}/events` (`list_dpp_events`). **Permission:** `dpp_history`.

### transpareo dpps get

Read a DPP

```
transpareo dpps get <id>
```

Example:

```
transpareo dpps get <id> --version <version>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--version` \<n\> | Return the historical signed snapshot for this version number instead of the passport as it stands. |

**API:** `GET /dpps/{id}` (`get_dpp`). **Permission:** `dpp_read`.

### transpareo dpps list

List Digital Product Passports

```
transpareo dpps list
```

Example:

```
transpareo dpps list --term <term>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--page` \<n\> | Page number |
| `--per-page` \<n\> | Records per page (default: 100, max: 500) |
| `--term` \<value\> | Search DPPs by code or description |

**API:** `GET /dpps` (`list_dpps`). **Permission:** `dpp_read`.

### transpareo dpps private-properties

Read the private properties of several versions

```
transpareo dpps private-properties <code>
```

Example:

```
transpareo dpps private-properties <code> --versions <versions>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--versions` \<value\> | Comma-separated version numbers ('versions[]=' is also accepted). At most 50 are derived. |

**API:** `GET /dpps/{code}/private_properties` (`get_dpp_private_properties`). **Permission:** `dpp_read`.

### transpareo dpps publish

Publish a DPP

```
transpareo dpps publish <code>
```

Example:

```
transpareo dpps publish <code> --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

**API:** `POST /dpps/{code}/publish` (`publish_dpp`). **Permission:** `dpp_write`.

### transpareo dpps reissue

Reissue a DPP

```
transpareo dpps reissue <id>
```

Example:

```
transpareo dpps reissue <id> --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

**API:** `POST /dpps/{id}/reissue` (`reissue_dpp`). **Permission:** `dpp_lifecycle`.

### transpareo dpps requirements

What a passport of a product needs

```
transpareo dpps requirements
```

Example:

```
transpareo dpps requirements --product-id <productId>
```

| Option | What it does |
|---|---|
| `--granularity` \<value\> | Which unit the passport would stand for. Defaults to the tenant's setting, which the answer repeats as 'defaultGranularity'. |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--product-id` \<n\> | The product a passport would describe. The snake_case spelling 'product_id' is accepted as well. |

**API:** `GET /dpps/requirements` (`get_dpp_requirements`). **Permission:** `dpp_read`.

### transpareo dpps stats

Get DPP scan statistics

```
transpareo dpps stats <id>
```

Example:

```
transpareo dpps stats <id> --format <format>
```

| Option | What it does |
|---|---|
| `--format` \<value\> | Response format |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

**API:** `GET /dpps/{id}/stats` (`get_dpp_stats`). **Permission:** `dpp_read`.

### transpareo dpps supersede

Supersede a DPP

```
transpareo dpps supersede <id>
```

Example:

```
transpareo dpps supersede <id> --file body.json --yes
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

**API:** `POST /dpps/{id}/supersede` (`supersede_dpp`). **Permission:** `dpp_lifecycle`. **Cannot be undone**; needs `--yes`.

### transpareo dpps update

Update a DPP

```
transpareo dpps update <id>
```

Example:

```
transpareo dpps update <id> --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

**API:** `PUT /dpps/{id}` (`update_dpp`). **Permission:** `dpp_write`.

### transpareo dpps validate

Validate a DPP payload

```
transpareo dpps validate
```

Example:

```
transpareo dpps validate --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

**API:** `POST /dpps/validate` (`validate_dpp`). **Permission:** `dpp_write`.

### transpareo dpps version-private-properties

Read the private properties of one version

```
transpareo dpps version-private-properties <code> <version>
```

Example:

```
transpareo dpps version-private-properties <code> <version>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

**API:** `GET /dpps/{code}/private_properties/{version}` (`get_dpp_version_private_properties`). **Permission:** `dpp_read`.

### transpareo dpps versions list

List the registered versions of a DPP

```
transpareo dpps versions list <id>
```

Example:

```
transpareo dpps versions list <id>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

**API:** `GET /dpps/{id}/versions` (`list_dpp_versions`). **Permission:** `dpp_read`.

### transpareo dpps void

Void a DPP

```
transpareo dpps void <id>
```

Example:

```
transpareo dpps void <id> --file body.json --yes
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

**API:** `POST /dpps/{id}/void` (`void_dpp`). **Permission:** `dpp_lifecycle`. **Cannot be undone**; needs `--yes`.

## Events

### transpareo events list

Poll the feed of passport events

```
transpareo events list
```

Example:

```
transpareo events list --since <since>
```

| Option | What it does |
|---|---|
| `--dpp-code` \<value\> | Confine the feed to the passport with this public code |
| `--limit` \<n\> | Events per answer |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--since` \<value\> | The 'nextCursor' of the previous answer, or an ISO 8601 time to start from |
| `--types` \<value\> | Comma-separated event types to keep, out of the 'eventType' values of 'DppEvent' |

**API:** `GET /events` (`list_events`). **Permission:** `dpp_history`.

### transpareo events tail

Read the workspace's passport events, optionally as a live stream

```
transpareo events tail [--since <cursor>] [--follow]
```

Example:

```
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

**API:** `GET /events` (`list_events`). **Permission:** `dpp_history`.

## Exports

### transpareo exports create

Start a passport export, wait for it and download the archive

```
transpareo exports create [--format jsonld|csv|xlsx|sql] [--wait] [--download <path>]
```

Example:

```
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

**API:** `POST /exports` (`create_export`). **Permission:** `export_access`.

### transpareo exports download

Download an export archive

```
transpareo exports download <id>
```

Example:

```
transpareo exports download <id>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

**API:** `GET /exports/{id}/download` (`download_export`). **Permission:** `export_access`.

### transpareo exports get

Poll an export

```
transpareo exports get <id>
```

Example:

```
transpareo exports get <id>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--wait` | poll the statusUrl until the work is done |

**API:** `GET /exports/{id}` (`get_export`). **Permission:** `export_access`.

## Grants

### transpareo grants create

Issue a passport-scoped partner grant

```
transpareo grants create
```

Example:

```
transpareo grants create --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

**API:** `POST /grant` (`create_grant`). **Permission:** `dpp_events` or `dpp_history` or `dpp_dynamic`.

## Imports

### transpareo imports create

Upload a spreadsheet to import

```
transpareo imports create
```

Example:

```
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

**API:** `POST /imports` (`create_import`). **Permission:** `import_access`.

### transpareo imports example

Download the example spreadsheet

```
transpareo imports example
```

Example:

```
transpareo imports example --data-type <dataType>
```

| Option | What it does |
|---|---|
| `--data-type` \<value\> |  |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

**API:** `GET /imports/example` (`get_import_example`). **Permission:** `import_access`.

### transpareo imports execute

Execute an import

```
transpareo imports execute <id>
```

Example:

```
transpareo imports execute <id> --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |
| `--wait` | poll the statusUrl until the work is done |

**API:** `POST /imports/{id}/execute` (`execute_import`). **Permission:** `import_access`.

### transpareo imports get

Poll an import

```
transpareo imports get <id>
```

Example:

```
transpareo imports get <id>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--wait` | poll the statusUrl until the work is done |

**API:** `GET /imports/{id}` (`get_import`). **Permission:** `import_access`.

### transpareo imports map

Send the column mapping of a fresh import

```
transpareo imports map <id>
```

Example:

```
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

**API:** `PUT /imports/{id}/mappings` (`map_import`). **Permission:** `import_access`.

### transpareo imports revert

Revert an import

```
transpareo imports revert <id>
```

Example:

```
transpareo imports revert <id> --file body.json --yes
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |
| `--wait` | poll the statusUrl until the work is done |

**API:** `POST /imports/{id}/revert` (`revert_import`). **Permission:** `import_access`. **Cannot be undone**; needs `--yes`.

### transpareo imports run

Upload, map, validate and, with --execute, run an import

```
transpareo imports run --file <path> --type <components|products|dpps>
```

Example:

```
transpareo imports run --file catalogue.xlsx --type products
transpareo imports run --file catalogue.xlsx --type products \
    --accept-suggestions --map "Farbe=new:Colour" --execute
```

| Option | What it does |
|---|---|
| `--accept-suggestions` | take every exact match of the preview |
| `--execute` | write the records when the validation passes |
| `--file` \<value\> | the spreadsheet or JSON file to import |
| `--map` \<value\>... | Column=target (repeatable) |
| `--mappings` \<value\> | mapping file (ImportMappingsInput) |
| `--published` | publish the records the import creates |
| `--skip-backup` | execute without the backup a revert needs |
| `--type` \<value\> | components, products or dpps |
| `--value-separator` \<value\> | what separates several values in a cell |

**API:** `POST /imports/{id}/execute` (`execute_import`). **Permission:** `import_access`.

### transpareo imports supplier-form

Download the blank supplier form

```
transpareo imports supplier-form
```

Example:

```
transpareo imports supplier-form --data-type <dataType>
```

| Option | What it does |
|---|---|
| `--data-type` \<value\> |  |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--sheet-locale` \<value\> | The language of the form; a language the workspace does not run falls back to the request's |

**API:** `GET /imports/supplier_form` (`get_import_supplier_form`). **Permission:** `import_access`.

### transpareo imports validate

Validate an import

```
transpareo imports validate <id>
```

Example:

```
transpareo imports validate <id>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--wait` | poll the statusUrl until the work is done |

**API:** `POST /imports/{id}/validate` (`validate_import`). **Permission:** `import_access`.

## Mediafiles

### transpareo mediafiles create

Upload a mediafile

```
transpareo mediafiles create
```

Example:

```
transpareo mediafiles create --file <path> --upload <path>
```

| Option | What it does |
|---|---|
| `--file` \<value\> | path of the file to upload; Image file to upload |
| `--name` \<value\> | Optional display name |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--upload` \<value\> | path of the file to upload; The same file under the name the model uses; send either this or 'mediafile[file]' |

**API:** `POST /mediafiles` (`create_mediafile`). **Permission:** any consumer token.

### transpareo mediafiles list

List mediafiles

```
transpareo mediafiles list
```

Example:

```
transpareo mediafiles list --page <page>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--page` \<n\> | Page number |
| `--per-page` \<n\> | Records per page (default: 100, max: 500) |
| `--term` \<value\> | Filter by file name |

**API:** `GET /mediafiles` (`list_mediafiles`). **Permission:** any consumer token.

## Permalinks

### transpareo permalinks resolve

Resolve a permalink

```
transpareo permalinks resolve <path>
```

Example:

```
transpareo permalinks resolve <path> --show <show>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--show` \<value\> | Comma-separated property type IDs to reveal restricted properties |

**API:** `GET /permalinks/{path}` (`resolve_permalink`). **Permission:** none, the endpoint is public.

## Plans

### transpareo plans get

Get a subscription plan

```
transpareo plans get <id>
```

Example:

```
transpareo plans get <id>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

**API:** `GET /plans/{id}` (`get_plan`). **Permission:** none, the endpoint is public.

### transpareo plans list

List subscription plans

```
transpareo plans list
```

Example:

```
transpareo plans list
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

**API:** `GET /plans` (`list_plans`). **Permission:** none, the endpoint is public.

## Products

### transpareo products create

Create a product

```
transpareo products create
```

Example:

```
transpareo products create --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

**API:** `POST /products` (`create_product`). **Permission:** `product_access`.

### transpareo products delete

Delete a product

```
transpareo products delete <id>
```

Example:

```
transpareo products delete <id> --yes
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

**API:** `DELETE /products/{id}` (`delete_product`). **Permission:** `product_access`. **Cannot be undone**; needs `--yes`.

### transpareo products featured list

List featured products

```
transpareo products featured list
```

Example:

```
transpareo products featured list
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

**API:** `GET /featured_products` (`list_featured_products`). **Permission:** none, the endpoint is public.

### transpareo products get

Get a product

```
transpareo products get <id>
```

Example:

```
transpareo products get <id> --show <show>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--raw` | Return raw property values instead of rendered HTML |
| `--show` \<value\> | Comma-separated property type IDs to reveal restricted properties |

**API:** `GET /products/{id}` (`get_product`). **Permission:** none, the endpoint is public.

### transpareo products list

List products

```
transpareo products list
```

Example:

```
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
| `--rating` \<value\> | Filter by rating (comma-separated values: A, B, C, D) |
| `--term` \<value\> | Full-text search query (uses Elasticsearch when provided) |
| `--type-ids` \<value\> | Filter by component type IDs (comma-separated) |

**API:** `GET /products` (`list_products`). **Permission:** none, the endpoint is public.

### transpareo products mediafiles update

Update product mediafile assignments

```
transpareo products mediafiles update <id>
```

Example:

```
transpareo products mediafiles update <id> --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

**API:** `PUT /products/{id}/mediafiles` (`update_product_mediafiles`). **Permission:** `product_access`.

### transpareo products new

Get new product template

```
transpareo products new
```

Example:

```
transpareo products new
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

**API:** `GET /products/new` (`get_new_product`). **Permission:** none, the endpoint is public.

### transpareo products properties list

List product properties

```
transpareo products properties list
```

Example:

```
transpareo products properties list --page <page>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--page` \<n\> | Page number |
| `--per-page` \<n\> | Records per page (default: 100, max: 500) |
| `--property-type-id` \<n\> | The property type whose values to list. Without it, and for a type that does not exist, the list is empty. |
| `--term` \<value\> | Filter the values by their text |

**API:** `GET /product_properties` (`list_product_properties`). **Permission:** none, the endpoint is public.

### transpareo products publish

Publish a product

```
transpareo products publish <id>
```

Example:

```
transpareo products publish <id>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

**API:** `PUT /products/{id}/publish` (`publish_product`). **Permission:** `product_access`.

### transpareo products unpublish

Unpublish a product

```
transpareo products unpublish <id>
```

Example:

```
transpareo products unpublish <id>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

**API:** `PUT /products/{id}/unpublish` (`unpublish_product`). **Permission:** `product_access`.

### transpareo products update

Update a product

```
transpareo products update <id>
```

Example:

```
transpareo products update <id> --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

**API:** `PUT /products/{id}` (`update_product`). **Permission:** `product_access`.

## Reference Data

### transpareo reference-data component-functions list

List component functions

```
transpareo reference-data component-functions list
```

Example:

```
transpareo reference-data component-functions list
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

**API:** `GET /component_functions` (`list_component_functions`). **Permission:** none, the endpoint is public.

### transpareo reference-data component-names list

Autocomplete component names

```
transpareo reference-data component-names list
```

Example:

```
transpareo reference-data component-names list --page <page>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--page` \<n\> | Page number |
| `--per-page` \<n\> | Records per page (default: 100, max: 500) |
| `--term` \<value\> | Prefix to search for |

**API:** `GET /component_names` (`list_component_names`). **Permission:** none, the endpoint is public.

### transpareo reference-data component-properties list

List component property categories

```
transpareo reference-data component-properties list
```

Example:

```
transpareo reference-data component-properties list
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

**API:** `GET /component_properties` (`list_component_properties`). **Permission:** none, the endpoint is public.

### transpareo reference-data component-types list

List component types

```
transpareo reference-data component-types list
```

Example:

```
transpareo reference-data component-types list
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

**API:** `GET /component_types` (`list_component_types`). **Permission:** none, the endpoint is public.

### transpareo reference-data countries list

List countries

```
transpareo reference-data countries list
```

Example:

```
transpareo reference-data countries list
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

**API:** `GET /countries` (`list_countries`). **Permission:** none, the endpoint is public.

### transpareo reference-data product-gtins list

Search products by GTIN

```
transpareo reference-data product-gtins list
```

Example:

```
transpareo reference-data product-gtins list --page <page>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--page` \<n\> | Page number |
| `--per-page` \<n\> | Records per page (default: 100, max: 500) |
| `--term` \<value\> | GTIN prefix to search for |

**API:** `GET /product_gtins` (`list_product_gtins`). **Permission:** none, the endpoint is public.

## Search

### transpareo search catalogue

Search products and components

```
transpareo search catalogue
```

Example:

```
transpareo search catalogue --query <query>
```

| Option | What it does |
|---|---|
| `--models` \<value\> | Comma-separated model types to search (e.g. 'products', 'components') |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--page` \<n\> | Page number |
| `--per-page` \<n\> | Records per page (default: 100, max: 500) |
| `--query` \<value\> | Search query |

**API:** `GET /search` (`search_catalogue`). **Permission:** none, the endpoint is public.

## Webhooks

### transpareo webhooks create

Create a webhook subscription

```
transpareo webhooks create
```

Example:

```
transpareo webhooks create --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

**API:** `POST /webhooks` (`create_webhook`). **Permission:** `webhook_access`.

### transpareo webhooks delete

Delete a webhook subscription

```
transpareo webhooks delete <id>
```

Example:

```
transpareo webhooks delete <id> --yes
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

**API:** `DELETE /webhooks/{id}` (`delete_webhook`). **Permission:** `webhook_access`. **Cannot be undone**; needs `--yes`.

### transpareo webhooks get

Get a webhook subscription

```
transpareo webhooks get <id>
```

Example:

```
transpareo webhooks get <id>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

**API:** `GET /webhooks/{id}` (`get_webhook`). **Permission:** `webhook_access`.

### transpareo webhooks list

List webhook subscriptions

```
transpareo webhooks list
```

Example:

```
transpareo webhooks list --page <page>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--page` \<n\> | Page number |
| `--per-page` \<n\> | Records per page (default: 100, max: 500) |

**API:** `GET /webhooks` (`list_webhooks`). **Permission:** `webhook_access`.

### transpareo webhooks secret regenerate

Rotate a webhook secret

```
transpareo webhooks secret regenerate <id>
```

Example:

```
transpareo webhooks secret regenerate <id> --yes
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

**API:** `POST /webhooks/{id}/regenerate_secret` (`regenerate_webhook_secret`). **Permission:** `webhook_access`. **Cannot be undone**; needs `--yes`.

### transpareo webhooks test

Send a test delivery

```
transpareo webhooks test <id>
```

Example:

```
transpareo webhooks test <id>
```

| Option | What it does |
|---|---|
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |

**API:** `POST /webhooks/{id}/test` (`test_webhook`). **Permission:** `webhook_access`.

### transpareo webhooks update

Update a webhook subscription

```
transpareo webhooks update <id>
```

Example:

```
transpareo webhooks update <id> --file body.json
```

| Option | What it does |
|---|---|
| `--file` \<value\> | request body from a file, or - for standard input |
| `-o`, `--output` \<value\> | write the answer to this file instead of standard output |
| `--set` \<value\>... | body field as key=value, nested with dots (repeatable) |

**API:** `PUT /webhooks/{id}` (`update_webhook`). **Permission:** `webhook_access`.

