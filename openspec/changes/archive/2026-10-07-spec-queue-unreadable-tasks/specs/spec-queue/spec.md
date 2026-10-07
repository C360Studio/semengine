# spec-queue

## ADDED Requirements

### Requirement: A tasks.md that cannot be read stops the queue and the check

When an in-flight change has a `tasks.md` file that `scripts/openspec-queue.sh` cannot open, or whose content is not
valid UTF-8, the script SHALL print on standard error the line `queue unavailable: cannot read <path>`, where `<path>`
is `openspec/changes/<name>/tasks.md`, and SHALL exit 2. This holds when the script prints the queue
(`task spec:queue`), with or without `--strict`, and when it runs with `--check` (`task spec:check`). The script SHALL
NOT print the `ok` line of requirement "Labels on a task's first line" for that change, and with `--check` it SHALL
NOT print `holds: ok`: a file the script did not read is never reported as free of caveats or of misplaced holds.
This requirement takes precedence over the `ok` and `holds: ok` lines and over the exit 0 and exit 1 that the
requirements above state, also when a change listed before that file has already reported a line. Other text, such
as the reader's own error, MAY come before that line on standard error. What the script printed before it reached
that file can already be on standard output when it exits: the queue's lines for the changes listed before it, and
that change's own count line, which the queue prints before it reads the change's `tasks.md`; or the
`<path>:<line>:` lines that `--check` printed for the changes listed before it.

#### Scenario: The queue meets a tasks.md that is not UTF-8

- **WHEN** the only in-flight change's `tasks.md` contains the two bytes `0xff 0xfe`, which are not valid UTF-8, and
  `scripts/openspec-queue.sh` is run without `--strict`
- **THEN** it prints `queue unavailable: cannot read openspec/changes/<name>/tasks.md` as a line on standard error,
  prints no `ok` line, and exits 2

#### Scenario: The check meets a tasks.md that is not UTF-8

- **WHEN** the only in-flight change's `tasks.md` contains the two bytes `0xff 0xfe`, and
  `scripts/openspec-queue.sh --check` is run
- **THEN** it prints `queue unavailable: cannot read openspec/changes/<name>/tasks.md` as a line on standard error,
  prints no `holds: ok`, and exits 2
