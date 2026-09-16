# MCP server

## The workspace's own endpoint

A workspace answers the Model Context Protocol itself, at
`https://<workspace host>/mcp` over the Streamable HTTP
transport, so an assistant that runs in a browser reaches the
same tools with nothing installed. Claude and ChatGPT take that
address as a connector: the endpoint publishes its OAuth
metadata at `/.well-known/oauth-protected-resource/mcp`, the
assistant discovers the workspace as the authorization server,
and the person approves a consent screen that names every
permission asked for. The OpenAI Agents SDK's hosted tool takes
the same address with a token. An approved connection is listed
on the API consumers page of the application manager, with the
name and date of whoever approved it, and is revoked there.

The tools are the ones below, under the same names, so nothing
in this document is a lesser set. Serve the protocol from this
binary instead when the assistant runs where no browser does, in
a script or offline, when the credential should stay on the
machine that stored it, or when the assistant should reach less
than the workspace allows, which is what `--read-only` and
`--tools` are for.

## From this binary

`transpareo mcp` serves the Model Context Protocol over standard
input and output, built on the official Go SDK. The assistant's
configuration holds no secret; the credential comes from the
profile the command line stored:

```json
{ "command": "transpareo", "args": ["mcp", "--profile", "acme"] }
```

Log output goes to standard error only, so it never mixes with the
protocol.

## Options

- `--profile <name>` selects the workspace, as everywhere.
- `--tools <group,...>` narrows the tools to groups: `products`
  (products, components, brands and the property types), `dpps`
  (passports, requirements, validation and bulk calls), `data`
  (imports, exports, tasks and the event feed, once those tools
  exist) and `webhooks`. `me`, `search_operations` and `call_api`
  are always present. A catalogue assistant does not carry webhook
  tools.
- `--read-only` removes every tool that changes data and keeps the
  validations. `call_api` refuses writes in that mode too.

## Tools

Forty curated tools cover the flows, and two escape hatches reach
every other endpoint.

Which operations are curated, under what names, in which groups,
of which shape and behind which confirm phrase is one declaration,
published by the workspace in its tool catalogue. This server
generates its table from the vendored copy of that document, so
the tools it serves and the tools a workspace's own endpoint
serves cannot describe the API differently. Each side still builds
the description and the schemas from that declaration in its own
code, and a test compares what the two buildings produced.

- Identity: `me`.
- Discovery: `search_operations(query)` finds operations by words
  in their id, summary, description or path and answers the
  operationId, method, path, parameters, permission and an example
  body; `call_api(operationId, path, query, body, confirm)` runs
  any of them.
- Products: `list_products`, `get_product`, `create_product`,
  `update_product`, `publish_product`, `unpublish_product`,
  `product_property_types`, `list_components`, `get_component`,
  `create_component`, `update_component`, `list_mediafiles`,
  `create_mediafile`, `update_product_mediafiles`, `list_brands`,
  `create_brand`.
- Passports: `dpp_requirements`, `list_dpps`, `get_dpp`,
  `validate_dpp`, `create_dpp`, `update_dpp`, `publish_dpp`,
  `append_dpp_event`, `update_dynamic_data`, `void_dpp`,
  `supersede_dpp`, `reissue_dpp`, `bulk_validate_dpps`,
  `bulk_create_dpps`.
- Webhooks: `list_webhooks`, `create_webhook`, `test_webhook`,
  `regenerate_webhook_secret`, `delete_webhook`. The create and
  regenerate tools answer the signing secret, because nothing
  else can; `--tools` or `--read-only` keeps them away from
  assistants that should not see it.

Every tool description carries the permission key, a note when the
action cannot be undone, the highest data tier the answer can
contain (public, authorised, restricted) and one example call, all
taken from the API specification. The protocol annotations follow
the specification too: read-only for reads and validations,
destructive for operations that cannot be undone, idempotent for
updates.

Every result is a one-line text summary plus structured content
with a declared output schema. A list answers `{items, page,
total, nextPage}`; a get answers the full record; a `fields`
argument narrows either.

A write whose operation replays an answer takes an
`idempotencyKey`. Repeating the call with the same key within a
day answers the result of the first one instead of writing again,
so a create that timed out can be sent a second time without
making a second record. Without a key the client still sends one,
freshly made per call, which covers a retry inside the call but
not a repeat of it.

## Safety

Tools that cannot be undone are present, not hidden, and take a
`confirm` argument that must equal the phrase the description
states, such as `void A1B2C3D4E`. The model has to write the
intent out, and a host that shows tool calls shows it to the user.
`call_api` applies the same rule with the phrase
`<operationId> <path value>`.

The credential decides what the assistant can reach. For an
assistant that should only read, create a consumer with read
permissions in the application manager, log in with it under its
own profile and start the server with `--profile` and
`--read-only`:

```
echo "$SECRET" | transpareo auth login --host acme.example.com \
    --client-id <key> --name acme-assistant
transpareo mcp --profile acme-assistant --read-only
```

`transpareo auth token --scope dpp_read` mints a token for a
subprocess that should never see the secret.

## Resources

- `transpareo://guide`: the workspace's API guide as Markdown.
- `transpareo://openapi`: the API specification the server was
  built with.
- `transpareo://me`: what the credential allows.

## Instructions

On connect the server tells the assistant to start with `me`; to
call `product_property_types` before `create_product`; for
passports of an existing product to call `dpp_requirements`, then
`validate_dpp` before `create_dpp`; that `publish_dpp` signs and
cannot be undone; that `void_dpp` and `supersede_dpp` need
`confirm`; to use `search_operations` then `call_api` for anything
without a tool; and that lists are paged.

## Coverage

A test keeps the curated set complete: every operation of the
specification is behind a tool, listed with a reason as reachable
through `call_api` only, a storefront flow no consumer token can
use, or a user-only operation. A new operation in the
specification fails the test until it is classified.
