# Command reference

Generated from specification 1.6.0 by `go generate ./...`; do not edit by hand.

Every command accepts `--json`, `--jsonl`, `-q`, `--fields`, `--profile`, `--read-only` and `--yes`. See the README for what they do.

## General

### `transpareo api <METHOD> <path>`

Send an authenticated request to any endpoint

```
transpareo api GET /dpps --query page=2 --query per_page=50
transpareo api POST /dpps/validate --body @passport.json
transpareo api PUT /products/42 --body '{"product": {"name": "New name"}}'
transpareo api DELETE /webhooks/7 --yes
```

- `--body` `<string>`: request body: JSON, @<file>, or - for stdin
- `--header` `<stringArray>`: extra header as Name: value (repeatable)
- `--idempotency-key` `<string>`: Idempotency-Key for a POST (default: random)
- `--query` `<stringArray>`: query parameter as name=value (repeatable)

### `transpareo auth grant --code <passport code>`

Issue a child credential scoped to one passport (POST /grant)

```
transpareo auth grant --code A1B2C3D4E
transpareo auth grant --gtin 04012345678901 --serial 000412
```

- `--batch` `<string>`: batch identifier, with --gtin
- `--code` `<string>`: the passport code printed on the QR code
- `--gtin` `<string>`: GTIN of the product, for a lookup by GS1 identifiers
- `--serial` `<string>`: serial identifier, with --gtin

Operation `create_grant`, `POST /grant`. Permission: `dpp_events` or `dpp_history` or `dpp_dynamic`.

### `transpareo auth login --host <workspace host> --client-id <key>`

Store a credential after checking it at the token endpoint

```
echo "$SECRET" | transpareo auth login \
    --host acme.example.com --client-id 3f6a...
transpareo auth login --host acme.example.com --client-id 3f6a... \
    --scope dpp_read,dpp_write --name acme-read
```

- `--client-id` `<string>`: the consumer's key
- `--default`: make this the default profile
- `--host` `<string>`: workspace host, such as acme.example.com
- `--name` `<string>`: profile name (default: the host)
- `--scope` `<string>`: permission keys to narrow tokens to, comma separated

Operation `exchange_token`, `POST /oauth/token`. No permission needed; the endpoint is public.

### `transpareo auth logout`

Remove the stored profile and its secret

### `transpareo auth status`

Show what the current credential allows (GET /me)

Operation `get_me`, `GET /me`. Any consumer token.

### `transpareo auth token [--scope a,b]`

Print a fresh short-lived bearer token for scripts and subagents

```
export TRANSPAREO_TOKEN=$(transpareo auth token \
    --scope dpp_read)
```

- `--scope` `<string>`: permission keys to narrow the token to, comma separated

Operation `exchange_token`, `POST /oauth/token`. No permission needed; the endpoint is public.

### `transpareo commands`

List every command and every API operation, for agents

```
transpareo commands --json | jq '.operations[] | .operationId'
```

### `transpareo doctor`

Check the setup: profile, host, token endpoint, credential, keyring

```
transpareo doctor
transpareo doctor --json
```

### `transpareo guide`

Print the workspace's API guide as Markdown

```
transpareo guide | less
```

### `transpareo me`

Show what the current credential allows (GET /me)

```
transpareo me
transpareo me --json
```

Operation `get_me`, `GET /me`. Any consumer token.

### `transpareo schema <operationId>`

Print the request schema and example of an operation

```
transpareo schema create_dpp
transpareo schema list_dpps --fields queryParams
```

### `transpareo version`

Print the version of the binary and of its API specification

## Brands

### `transpareo brands create`

Create a brand

```
transpareo brands create --file body.json
```

- `--file` `<string>`: request body from a file, or - for standard input
- `--output` `<string>`: write the answer to this file instead of standard output
- `--set` `<stringArray>`: body field as key=value, nested with dots (repeatable)

Operation `create_brand`, `POST /brands`. Permission: `brand_access` or `brand_write`.

### `transpareo brands delete <id>`

Delete a brand

```
transpareo brands delete <id> --yes
```

- `--output` `<string>`: write the answer to this file instead of standard output

Operation `delete_brand`, `DELETE /brands/{id}`. Permission: `brand_access` or `brand_write`. Cannot be undone; needs `--yes`.

### `transpareo brands get <id>`

Get a brand

```
transpareo brands get <id>
```

- `--output` `<string>`: write the answer to this file instead of standard output

Operation `get_brand`, `GET /brands/{id}`. No permission needed; the endpoint is public.

### `transpareo brands list`

List brands

```
transpareo brands list --page <page>
```

- `--output` `<string>`: write the answer to this file instead of standard output
- `--page` `<int>`: Page number
- `--per-page` `<int>`: Records per page (default: 100, max: 500)

Operation `list_brands`, `GET /brands`. No permission needed; the endpoint is public.

### `transpareo brands update <id>`

Rename a brand

```
transpareo brands update <id> --file body.json
```

- `--file` `<string>`: request body from a file, or - for standard input
- `--output` `<string>`: write the answer to this file instead of standard output
- `--set` `<stringArray>`: body field as key=value, nested with dots (repeatable)

Operation `update_brand`, `PUT /brands/{id}`. Permission: `brand_access` or `brand_write`.

## Categories

### `transpareo categories list`

List product categories

```
transpareo categories list
```

- `--output` `<string>`: write the answer to this file instead of standard output

Operation `list_product_categories`, `GET /product_categories`. No permission needed; the endpoint is public.

## Components

### `transpareo components create`

Create a component

```
transpareo components create --file body.json
```

- `--file` `<string>`: request body from a file, or - for standard input
- `--output` `<string>`: write the answer to this file instead of standard output
- `--set` `<stringArray>`: body field as key=value, nested with dots (repeatable)

Operation `create_component`, `POST /components`. Permission: `component_access` or `component_write`.

### `transpareo components delete <id>`

Delete a component

```
transpareo components delete <id> --yes
```

- `--output` `<string>`: write the answer to this file instead of standard output

Operation `delete_component`, `DELETE /components/{id}`. Permission: `component_access` or `component_write`. Cannot be undone; needs `--yes`.

### `transpareo components get <id>`

Get a component

```
transpareo components get <id>
```

- `--output` `<string>`: write the answer to this file instead of standard output

Operation `get_component`, `GET /components/{id}`. No permission needed; the endpoint is public.

### `transpareo components list`

List components

```
transpareo components list --page <page>
```

- `--function-ids` `<string>`: Filter by function IDs (comma-separated)
- `--output` `<string>`: write the answer to this file instead of standard output
- `--page` `<int>`: Page number
- `--per-page` `<int>`: Records per page (default: 100, max: 500)
- `--property-category-ids` `<string>`: Filter by property category IDs (comma-separated)
- `--term` `<string>`: Full-text search query
- `--type-ids` `<string>`: Filter by component type IDs (comma-separated)

Operation `list_components`, `GET /components`. No permission needed; the endpoint is public.

### `transpareo components publish <id>`

Publish a component

```
transpareo components publish <id>
```

- `--output` `<string>`: write the answer to this file instead of standard output

Operation `publish_component`, `PUT /components/{id}/publish`. Permission: `component_access` or `component_write`.

### `transpareo components unpublish <id>`

Unpublish a component

```
transpareo components unpublish <id>
```

- `--output` `<string>`: write the answer to this file instead of standard output

Operation `unpublish_component`, `PUT /components/{id}/unpublish`. Permission: `component_access` or `component_write`.

### `transpareo components update <id>`

Update a component

```
transpareo components update <id> --file body.json
```

- `--file` `<string>`: request body from a file, or - for standard input
- `--output` `<string>`: write the answer to this file instead of standard output
- `--set` `<stringArray>`: body field as key=value, nested with dots (repeatable)

Operation `update_component`, `PUT /components/{id}`. Permission: `component_access` or `component_write`.

## Configuration

### `transpareo configuration config`

Get tenant configuration

```
transpareo configuration config
```

- `--output` `<string>`: write the answer to this file instead of standard output

Operation `get_config`, `GET /config`. No permission needed; the endpoint is public.

### `transpareo configuration languages list`

List available languages

```
transpareo configuration languages list
```

- `--output` `<string>`: write the answer to this file instead of standard output

Operation `list_languages`, `GET /languages`. No permission needed; the endpoint is public.

### `transpareo configuration localization`

Get localization strings

```
transpareo configuration localization
```

- `--output` `<string>`: write the answer to this file instead of standard output

Operation `get_localization`, `GET /localization`. No permission needed; the endpoint is public.

### `transpareo configuration navigation`

Get navigation items

```
transpareo configuration navigation
```

- `--output` `<string>`: write the answer to this file instead of standard output

Operation `get_navigation`, `GET /navigation`. No permission needed; the endpoint is public.

## Coupons

### `transpareo coupons lookup`

Look up a coupon by code

```
transpareo coupons lookup --code <code>
```

- `--code` `<string>`: Coupon code (case-insensitive)
- `--currency` `<string>`: Currency code to filter by (e.g. EUR, USD)
- `--output` `<string>`: write the answer to this file instead of standard output

Operation `lookup_coupon`, `GET /coupons`. No permission needed; the endpoint is public.

## DPPs

### `transpareo dpps bulk create`

Create DPPs in bulk

```
transpareo dpps bulk create --file body.json
```

- `--file` `<string>`: request body from a file, or - for standard input
- `--output` `<string>`: write the answer to this file instead of standard output
- `--prefer` `<string>`: Send 'respond-async' to process the batch in the background and answer 202 with a polling URL.
- `--wait`: poll the statusUrl until the work is done

Operation `bulk_create_dpps`, `POST /dpps/bulk`. Permission: `dpp_bulk_write`.

### `transpareo dpps bulk validate`

Validate DPPs in bulk

```
transpareo dpps bulk validate --file body.json
```

- `--file` `<string>`: request body from a file, or - for standard input
- `--output` `<string>`: write the answer to this file instead of standard output

Operation `validate_dpps_bulk`, `POST /dpps/bulk/validate`. Permission: `dpp_bulk_write`.

### `transpareo dpps bulk-task <taskId>`

Poll an asynchronous bulk create

```
transpareo dpps bulk-task <taskId>
```

- `--output` `<string>`: write the answer to this file instead of standard output

Operation `get_bulk_task`, `GET /dpps/bulk/{taskId}`. Permission: `dpp_bulk_write`.

### `transpareo dpps create`

Create a DPP

```
transpareo dpps create --file body.json
```

- `--file` `<string>`: request body from a file, or - for standard input
- `--output` `<string>`: write the answer to this file instead of standard output
- `--set` `<stringArray>`: body field as key=value, nested with dots (repeatable)

Operation `create_dpp`, `POST /dpps`. Permission: `dpp_write`.

### `transpareo dpps delete <id>`

Delete a DPP

```
transpareo dpps delete <id> --yes
```

- `--output` `<string>`: write the answer to this file instead of standard output

Operation `delete_dpp`, `DELETE /dpps/{id}`. Permission: `dpp_write`. Cannot be undone; needs `--yes`.

### `transpareo dpps dynamic-data update <id>`

Update dynamic data

```
transpareo dpps dynamic-data update <id> --file body.json
```

- `--file` `<string>`: request body from a file, or - for standard input
- `--output` `<string>`: write the answer to this file instead of standard output
- `--set` `<stringArray>`: body field as key=value, nested with dots (repeatable)

Operation `update_dpp_dynamic_data`, `PATCH /dpps/{id}/dynamic_data`. Permission: `dpp_dynamic`.

### `transpareo dpps events append <id>`

Append an event to a DPP

```
transpareo dpps events append <id> --file body.json
```

- `--file` `<string>`: request body from a file, or - for standard input
- `--output` `<string>`: write the answer to this file instead of standard output
- `--set` `<stringArray>`: body field as key=value, nested with dots (repeatable)

Operation `append_dpp_event`, `POST /dpps/{id}/events`. Permission: `dpp_events`.

### `transpareo dpps events list <id>`

List the event log of a DPP

```
transpareo dpps events list <id> --version <version>
```

- `--output` `<string>`: write the answer to this file instead of standard output
- `--version` `<int>`: Return only the events sealed into this version

Operation `list_dpp_events`, `GET /dpps/{id}/events`. Permission: `dpp_history`.

### `transpareo dpps get <id>`

Download DPP QR code

```
transpareo dpps get <id> --format <format>
```

- `--format` `<string>`: QR media format (also selectable via .png / .pdf extension). Default png.
- `--output` `<string>`: write the answer to this file instead of standard output
- `--version` `<int>`: Return the historical signed snapshot for this version number as JSON instead of QR media.

Operation `get_dpp`, `GET /dpps/{id}`. Permission: `dpp_read`.

### `transpareo dpps list`

List Digital Product Passports

```
transpareo dpps list --term <term>
```

- `--output` `<string>`: write the answer to this file instead of standard output
- `--page` `<int>`: Page number
- `--per-page` `<int>`: Records per page (default: 100, max: 500)
- `--term` `<string>`: Search DPPs by code or description

Operation `list_dpps`, `GET /dpps`. Permission: `dpp_read`.

### `transpareo dpps private-properties <code>`

Read the private properties of several versions

```
transpareo dpps private-properties <code> --versions <versions>
```

- `--output` `<string>`: write the answer to this file instead of standard output
- `--versions` `<string>`: Comma-separated version numbers ('versions[]=' is also accepted). At most 50 are derived.

Operation `get_dpp_private_properties`, `GET /dpps/{code}/private_properties`. Permission: `dpp_read`.

### `transpareo dpps publish <code>`

Publish a DPP

```
transpareo dpps publish <code> --file body.json
```

- `--file` `<string>`: request body from a file, or - for standard input
- `--output` `<string>`: write the answer to this file instead of standard output
- `--set` `<stringArray>`: body field as key=value, nested with dots (repeatable)

Operation `publish_dpp`, `POST /dpps/{code}/publish`. Permission: `dpp_write`.

### `transpareo dpps reissue <id>`

Reissue a DPP

```
transpareo dpps reissue <id> --file body.json
```

- `--file` `<string>`: request body from a file, or - for standard input
- `--output` `<string>`: write the answer to this file instead of standard output
- `--set` `<stringArray>`: body field as key=value, nested with dots (repeatable)

Operation `reissue_dpp`, `POST /dpps/{id}/reissue`. Permission: `dpp_lifecycle`.

### `transpareo dpps requirements`

What a passport of a product needs

```
transpareo dpps requirements --product-id <productId>
```

- `--granularity` `<string>`: Which unit the passport would stand for. Defaults to the tenant's setting, which the answer repeats as 'defaultGranularity'.
- `--output` `<string>`: write the answer to this file instead of standard output
- `--product-id` `<string>`: The product a passport would describe. The snake_case spelling 'product_id' is accepted as well.

Operation `get_dpp_requirements`, `GET /dpps/requirements`. Permission: `dpp_read`.

### `transpareo dpps stats <id>`

Get DPP scan statistics

```
transpareo dpps stats <id> --format <format>
```

- `--format` `<string>`: Response format
- `--output` `<string>`: write the answer to this file instead of standard output

Operation `get_dpp_stats`, `GET /dpps/{id}/stats`. Permission: `dpp_read`.

### `transpareo dpps supersede <id>`

Supersede a DPP

```
transpareo dpps supersede <id> --file body.json --yes
```

- `--file` `<string>`: request body from a file, or - for standard input
- `--output` `<string>`: write the answer to this file instead of standard output
- `--set` `<stringArray>`: body field as key=value, nested with dots (repeatable)

Operation `supersede_dpp`, `POST /dpps/{id}/supersede`. Permission: `dpp_lifecycle`. Cannot be undone; needs `--yes`.

### `transpareo dpps update <id>`

Update a DPP

```
transpareo dpps update <id> --file body.json
```

- `--file` `<string>`: request body from a file, or - for standard input
- `--output` `<string>`: write the answer to this file instead of standard output
- `--set` `<stringArray>`: body field as key=value, nested with dots (repeatable)

Operation `update_dpp`, `PUT /dpps/{id}`. Permission: `dpp_write`.

### `transpareo dpps validate`

Validate a DPP payload

```
transpareo dpps validate --file body.json
```

- `--file` `<string>`: request body from a file, or - for standard input
- `--output` `<string>`: write the answer to this file instead of standard output
- `--set` `<stringArray>`: body field as key=value, nested with dots (repeatable)

Operation `validate_dpp`, `POST /dpps/validate`. Permission: `dpp_write`.

### `transpareo dpps version-private-properties <code> <version>`

Read the private properties of one version

```
transpareo dpps version-private-properties <code> <version>
```

- `--output` `<string>`: write the answer to this file instead of standard output

Operation `get_dpp_version_private_properties`, `GET /dpps/{code}/private_properties/{version}`. Permission: `dpp_read`.

### `transpareo dpps versions list <id>`

List the registered versions of a DPP

```
transpareo dpps versions list <id>
```

- `--output` `<string>`: write the answer to this file instead of standard output

Operation `list_dpp_versions`, `GET /dpps/{id}/versions`. Permission: `dpp_read`.

### `transpareo dpps void <id>`

Void a DPP

```
transpareo dpps void <id> --file body.json --yes
```

- `--file` `<string>`: request body from a file, or - for standard input
- `--output` `<string>`: write the answer to this file instead of standard output
- `--set` `<stringArray>`: body field as key=value, nested with dots (repeatable)

Operation `void_dpp`, `POST /dpps/{id}/void`. Permission: `dpp_lifecycle`. Cannot be undone; needs `--yes`.

## Events

### `transpareo events list`

Poll the feed of passport events

```
transpareo events list --since <since>
```

- `--dpp-code` `<string>`: Confine the feed to the passport with this public code
- `--limit` `<int>`: Events per answer
- `--output` `<string>`: write the answer to this file instead of standard output
- `--since` `<string>`: The 'nextCursor' of the previous answer, or an ISO 8601 time to start from
- `--types` `<string>`: Comma-separated event types to keep, out of the 'eventType' values of 'DppEvent'

Operation `list_events`, `GET /events`. Permission: `dpp_history`.

## Exports

### `transpareo exports create`

Start a passport export

```
transpareo exports create --file body.json
```

- `--file` `<string>`: request body from a file, or - for standard input
- `--output` `<string>`: write the answer to this file instead of standard output
- `--set` `<stringArray>`: body field as key=value, nested with dots (repeatable)
- `--wait`: poll the statusUrl until the work is done

Operation `create_export`, `POST /exports`. Permission: `export_access`.

### `transpareo exports download <id>`

Download an export archive

```
transpareo exports download <id>
```

- `--output` `<string>`: write the answer to this file instead of standard output

Operation `download_export`, `GET /exports/{id}/download`. Permission: `export_access`.

### `transpareo exports get <id>`

Poll an export

```
transpareo exports get <id>
```

- `--output` `<string>`: write the answer to this file instead of standard output
- `--wait`: poll the statusUrl until the work is done

Operation `get_export`, `GET /exports/{id}`. Permission: `export_access`.

## Grants

### `transpareo grants create`

Issue a passport-scoped partner grant

```
transpareo grants create --file body.json
```

- `--file` `<string>`: request body from a file, or - for standard input
- `--output` `<string>`: write the answer to this file instead of standard output
- `--set` `<stringArray>`: body field as key=value, nested with dots (repeatable)

Operation `create_grant`, `POST /grant`. Permission: `dpp_events` or `dpp_history` or `dpp_dynamic`.

## Imports

### `transpareo imports create`

Upload a spreadsheet to import

```
transpareo imports create --file <path>
```

- `--data-type` `<string>`
- `--file` `<string>`: path of the file to upload; The spreadsheet, at most 50 MB, 100000 rows and 2000000 cells
- `--mappings` `<string>`: The mapping of 'ImportMappingsInput', sent as nested form fields (JSON)
- `--options` `<string>`: The options of 'ImportMappingsInput', sent as nested form fields (JSON)
- `--output` `<string>`: write the answer to this file instead of standard output
- `--value-separator` `<string>`: What separates several values in one cell
- `--wait`: poll the statusUrl until the work is done

Operation `create_import`, `POST /imports`. Permission: `import_access`.

### `transpareo imports example`

Download the example spreadsheet

```
transpareo imports example --data-type <dataType>
```

- `--data-type` `<string>`
- `--output` `<string>`: write the answer to this file instead of standard output

Operation `get_import_example`, `GET /imports/example`. Permission: `import_access`.

### `transpareo imports execute <id>`

Execute an import

```
transpareo imports execute <id> --file body.json
```

- `--file` `<string>`: request body from a file, or - for standard input
- `--output` `<string>`: write the answer to this file instead of standard output
- `--set` `<stringArray>`: body field as key=value, nested with dots (repeatable)
- `--wait`: poll the statusUrl until the work is done

Operation `execute_import`, `POST /imports/{id}/execute`. Permission: `import_access`.

### `transpareo imports get <id>`

Poll an import

```
transpareo imports get <id>
```

- `--output` `<string>`: write the answer to this file instead of standard output
- `--wait`: poll the statusUrl until the work is done

Operation `get_import`, `GET /imports/{id}`. Permission: `import_access`.

### `transpareo imports map <id>`

Map the columns of an import

```
transpareo imports map <id> --file body.json
```

- `--file` `<string>`: request body from a file, or - for standard input
- `--output` `<string>`: write the answer to this file instead of standard output
- `--set` `<stringArray>`: body field as key=value, nested with dots (repeatable)
- `--wait`: poll the statusUrl until the work is done

Operation `map_import`, `PUT /imports/{id}/mappings`. Permission: `import_access`.

### `transpareo imports revert <id>`

Revert an import

```
transpareo imports revert <id> --file body.json --yes
```

- `--file` `<string>`: request body from a file, or - for standard input
- `--output` `<string>`: write the answer to this file instead of standard output
- `--set` `<stringArray>`: body field as key=value, nested with dots (repeatable)
- `--wait`: poll the statusUrl until the work is done

Operation `revert_import`, `POST /imports/{id}/revert`. Permission: `import_access`. Cannot be undone; needs `--yes`.

### `transpareo imports supplier-form`

Download the blank supplier form

```
transpareo imports supplier-form --data-type <dataType>
```

- `--data-type` `<string>`
- `--output` `<string>`: write the answer to this file instead of standard output
- `--sheet-locale` `<string>`: The language of the form; a language the workspace does not run falls back to the request's

Operation `get_import_supplier_form`, `GET /imports/supplier_form`. Permission: `import_access`.

### `transpareo imports validate <id>`

Validate an import

```
transpareo imports validate <id>
```

- `--output` `<string>`: write the answer to this file instead of standard output
- `--wait`: poll the statusUrl until the work is done

Operation `validate_import`, `POST /imports/{id}/validate`. Permission: `import_access`.

## Mediafiles

### `transpareo mediafiles create`

Upload a mediafile

```
transpareo mediafiles create --file <path>
```

- `--file` `<string>`: path of the file to upload; Image file to upload
- `--name` `<string>`: Optional display name
- `--output` `<string>`: write the answer to this file instead of standard output

Operation `create_mediafile`, `POST /mediafiles`. Any consumer token.

### `transpareo mediafiles list`

List mediafiles

```
transpareo mediafiles list --page <page>
```

- `--output` `<string>`: write the answer to this file instead of standard output
- `--page` `<int>`: Page number
- `--per-page` `<int>`: Records per page (default: 100, max: 500)
- `--term` `<string>`: Filter by file name

Operation `list_mediafiles`, `GET /mediafiles`. Any consumer token.

## Permalinks

### `transpareo permalinks resolve <path>`

Resolve a permalink

```
transpareo permalinks resolve <path> --show <show>
```

- `--output` `<string>`: write the answer to this file instead of standard output
- `--show` `<string>`: Comma-separated property type IDs to reveal restricted properties

Operation `resolve_permalink`, `GET /permalinks/{path}`. No permission needed; the endpoint is public.

## Plans

### `transpareo plans get <id>`

Get a subscription plan

```
transpareo plans get <id>
```

- `--output` `<string>`: write the answer to this file instead of standard output

Operation `get_plan`, `GET /plans/{id}`. No permission needed; the endpoint is public.

### `transpareo plans list`

List subscription plans

```
transpareo plans list
```

- `--output` `<string>`: write the answer to this file instead of standard output

Operation `list_plans`, `GET /plans`. No permission needed; the endpoint is public.

## Products

### `transpareo products create`

Create a product

```
transpareo products create --file body.json
```

- `--file` `<string>`: request body from a file, or - for standard input
- `--output` `<string>`: write the answer to this file instead of standard output
- `--set` `<stringArray>`: body field as key=value, nested with dots (repeatable)

Operation `create_product`, `POST /products`. Permission: `product_access`.

### `transpareo products delete <id>`

Delete a product

```
transpareo products delete <id> --yes
```

- `--output` `<string>`: write the answer to this file instead of standard output

Operation `delete_product`, `DELETE /products/{id}`. Permission: `product_access`. Cannot be undone; needs `--yes`.

### `transpareo products featured list`

List featured products

```
transpareo products featured list
```

- `--output` `<string>`: write the answer to this file instead of standard output

Operation `list_featured_products`, `GET /featured_products`. No permission needed; the endpoint is public.

### `transpareo products get <id>`

Get a product

```
transpareo products get <id> --show <show>
```

- `--output` `<string>`: write the answer to this file instead of standard output
- `--raw`: Return raw property values instead of rendered HTML
- `--show` `<string>`: Comma-separated property type IDs to reveal restricted properties

Operation `get_product`, `GET /products/{id}`. No permission needed; the endpoint is public.

### `transpareo products list`

List products

```
transpareo products list --page <page>
```

- `--brand-id` `<string>`: Filter by brand ID
- `--category-ids` `<string>`: Filter by category IDs (comma-separated). Includes all child categories.
- `--exact`: When true with 'term', match exact product name
- `--function-ids` `<string>`: Filter by component function IDs (comma-separated)
- `--output` `<string>`: write the answer to this file instead of standard output
- `--page` `<int>`: Page number
- `--per-page` `<int>`: Records per page (default: 100, max: 500)
- `--property-category-ids` `<string>`: Filter by component property category IDs (comma-separated)
- `--rating` `<string>`: Filter by rating (comma-separated values: A, B, C, D)
- `--term` `<string>`: Full-text search query (uses Elasticsearch when provided)
- `--type-ids` `<string>`: Filter by component type IDs (comma-separated)

Operation `list_products`, `GET /products`. No permission needed; the endpoint is public.

### `transpareo products mediafiles update <id>`

Update product mediafile assignments

```
transpareo products mediafiles update <id> --file body.json
```

- `--file` `<string>`: request body from a file, or - for standard input
- `--output` `<string>`: write the answer to this file instead of standard output
- `--set` `<stringArray>`: body field as key=value, nested with dots (repeatable)

Operation `update_product_mediafiles`, `PUT /products/{id}/mediafiles`. Permission: `product_access`.

### `transpareo products new`

Get new product template

```
transpareo products new
```

- `--output` `<string>`: write the answer to this file instead of standard output

Operation `get_new_product`, `GET /products/new`. No permission needed; the endpoint is public.

### `transpareo products properties list`

List product properties

```
transpareo products properties list --page <page>
```

- `--output` `<string>`: write the answer to this file instead of standard output
- `--page` `<int>`: Page number
- `--per-page` `<int>`: Records per page (default: 100, max: 500)

Operation `list_product_properties`, `GET /product_properties`. No permission needed; the endpoint is public.

### `transpareo products publish <id>`

Publish a product

```
transpareo products publish <id>
```

- `--output` `<string>`: write the answer to this file instead of standard output

Operation `publish_product`, `PUT /products/{id}/publish`. Permission: `product_access`.

### `transpareo products unpublish <id>`

Unpublish a product

```
transpareo products unpublish <id>
```

- `--output` `<string>`: write the answer to this file instead of standard output

Operation `unpublish_product`, `PUT /products/{id}/unpublish`. Permission: `product_access`.

### `transpareo products update <id>`

Update a product

```
transpareo products update <id> --file body.json
```

- `--file` `<string>`: request body from a file, or - for standard input
- `--output` `<string>`: write the answer to this file instead of standard output
- `--set` `<stringArray>`: body field as key=value, nested with dots (repeatable)

Operation `update_product`, `PUT /products/{id}`. Permission: `product_access`.

## Reference Data

### `transpareo reference-data component-functions list`

List component functions

```
transpareo reference-data component-functions list
```

- `--output` `<string>`: write the answer to this file instead of standard output

Operation `list_component_functions`, `GET /component_functions`. No permission needed; the endpoint is public.

### `transpareo reference-data component-names list`

Autocomplete component names

```
transpareo reference-data component-names list --page <page>
```

- `--output` `<string>`: write the answer to this file instead of standard output
- `--page` `<int>`: Page number
- `--per-page` `<int>`: Records per page (default: 100, max: 500)
- `--term` `<string>`: Prefix to search for

Operation `list_component_names`, `GET /component_names`. No permission needed; the endpoint is public.

### `transpareo reference-data component-properties list`

List component property categories

```
transpareo reference-data component-properties list
```

- `--output` `<string>`: write the answer to this file instead of standard output

Operation `list_component_properties`, `GET /component_properties`. No permission needed; the endpoint is public.

### `transpareo reference-data component-types list`

List component types

```
transpareo reference-data component-types list
```

- `--output` `<string>`: write the answer to this file instead of standard output

Operation `list_component_types`, `GET /component_types`. No permission needed; the endpoint is public.

### `transpareo reference-data countries list`

List countries

```
transpareo reference-data countries list
```

- `--output` `<string>`: write the answer to this file instead of standard output

Operation `list_countries`, `GET /countries`. No permission needed; the endpoint is public.

### `transpareo reference-data product-gtins list`

Search products by GTIN

```
transpareo reference-data product-gtins list --page <page>
```

- `--output` `<string>`: write the answer to this file instead of standard output
- `--page` `<int>`: Page number
- `--per-page` `<int>`: Records per page (default: 100, max: 500)
- `--term` `<string>`: GTIN prefix to search for

Operation `list_product_gtins`, `GET /product_gtins`. No permission needed; the endpoint is public.

## Search

### `transpareo search catalogue`

Search products and components

```
transpareo search catalogue --query <query>
```

- `--models` `<string>`: Comma-separated model types to search (e.g. 'products', 'components')
- `--output` `<string>`: write the answer to this file instead of standard output
- `--page` `<int>`: Page number
- `--per-page` `<int>`: Records per page (default: 100, max: 500)
- `--query` `<string>`: Search query

Operation `search_catalogue`, `GET /search`. No permission needed; the endpoint is public.

## Webhooks

### `transpareo webhooks create`

Create a webhook subscription

```
transpareo webhooks create --file body.json
```

- `--file` `<string>`: request body from a file, or - for standard input
- `--output` `<string>`: write the answer to this file instead of standard output
- `--set` `<stringArray>`: body field as key=value, nested with dots (repeatable)

Operation `create_webhook`, `POST /webhooks`. Permission: `webhook_access`.

### `transpareo webhooks delete <id>`

Delete a webhook subscription

```
transpareo webhooks delete <id> --yes
```

- `--output` `<string>`: write the answer to this file instead of standard output

Operation `delete_webhook`, `DELETE /webhooks/{id}`. Permission: `webhook_access`. Cannot be undone; needs `--yes`.

### `transpareo webhooks get <id>`

Get a webhook subscription

```
transpareo webhooks get <id>
```

- `--output` `<string>`: write the answer to this file instead of standard output

Operation `get_webhook`, `GET /webhooks/{id}`. Permission: `webhook_access`.

### `transpareo webhooks list`

List webhook subscriptions

```
transpareo webhooks list --page <page>
```

- `--output` `<string>`: write the answer to this file instead of standard output
- `--page` `<int>`: Page number
- `--per-page` `<int>`: Records per page (default: 100, max: 500)

Operation `list_webhooks`, `GET /webhooks`. Permission: `webhook_access`.

### `transpareo webhooks secret regenerate <id>`

Rotate a webhook secret

```
transpareo webhooks secret regenerate <id> --yes
```

- `--output` `<string>`: write the answer to this file instead of standard output

Operation `regenerate_webhook_secret`, `POST /webhooks/{id}/regenerate_secret`. Permission: `webhook_access`. Cannot be undone; needs `--yes`.

### `transpareo webhooks test <id>`

Send a test delivery

```
transpareo webhooks test <id>
```

- `--output` `<string>`: write the answer to this file instead of standard output

Operation `test_webhook`, `POST /webhooks/{id}/test`. Permission: `webhook_access`.

### `transpareo webhooks update <id>`

Update a webhook subscription

```
transpareo webhooks update <id> --file body.json
```

- `--file` `<string>`: request body from a file, or - for standard input
- `--output` `<string>`: write the answer to this file instead of standard output
- `--set` `<stringArray>`: body field as key=value, nested with dots (repeatable)

Operation `update_webhook`, `PUT /webhooks/{id}`. Permission: `webhook_access`.

