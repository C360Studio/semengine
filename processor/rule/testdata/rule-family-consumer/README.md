# Rule-family consumer fixtures

Rules a product writes for its own action type, `publish_agent`, whose settings (`tool_choice`, `response_format`)
the rule engine never reads. `TestBodyKeepsBytes` in `action_test.go` decodes each rule and checks that its action's
`Body` is the file's bytes for that action, and that those settings decode to the values in `expected.json`.

The files are copied unchanged from the semteams repository, <https://github.com/C360Studio/semteams>, at commit
`a331910ee6ec297f86530c4a1d69ca84001a19f5`, from `test/fixtures/rule-family-consumer/`:

| File | What it holds |
| --- | --- |
| `required-live.json` | A rule taken from one of semteams' live rules, with `tool_choice` mode `required` |
| `forced-function.json` | A rule whose `tool_choice` forces one function |
| `response-format.json` | A rule with a JSON-schema `response_format` |
| `expected.json` | The values each rule's settings must decode to, written by hand, not by a decoder |
| `provenance.json` | semteams' own record of where `required-live.json` came from |

semteams is MIT licensed (its `LICENSE` file at that commit, copyright 2025 C360). Do not edit the files: the test
compares bytes, so a change to a file is a change to what the test proves.
