# The agent run before a release

Automated checks prove that the tools work. This run, done by a
person before each release, checks that they are pleasant to use:
a scripted assistant session, with only the skill and the MCP
server, completes the five cheat-sheet flows on the test tenant,
and the transcript is read for friction.

## Setup

1. Build the release candidate and put it on the PATH.
2. Log in to the test tenant with the write-scoped consumer under
   its own profile: `transpareo auth login --host <test host>
   --client-id <key> --name e2e`.
3. `transpareo setup claude --profile e2e` (or `setup codex`).
4. Start the assistant in an empty directory and confirm the
   `transpareo` server and skill are loaded.

## The five flows

Give the assistant these tasks, one after the other, in plain
words, and do not help it:

1. "What does our credential on Transpareo allow?"
2. "Create a product called Test Cream with the component Aqua,
   then tell me what a unit passport of it needs."
3. "Validate and create a passport for serial 000001 of that
   product, then publish it."
4. "Void that passport, reason recalled." The assistant must ask
   or state the confirm phrase; a host that shows tool calls
   must show it.
5. "What happened to our passports today?" (the event feed),
   then "export the catalogue as CSV and tell me where the
   archive is."

Finish with: "Delete the product you created."

## What to read the transcript for

- A wrong tool picked, or `call_api` used where a tool exists.
- An error not understood: the hint was there and the assistant
  did not follow it.
- Extra round trips: a `product_property_types` or
  `dpp_requirements` call skipped and then needed.
- The confirm phrase invented rather than taken from the tool
  description.
- Anything the skill says that the assistant ignored.

Each finding becomes a change to the skill text, a tool
description or the server instructions, and the run is repeated.
Record the date, the assistant, the model and the findings in
`.todo/` of the release branch; nothing of the transcript is
committed.
