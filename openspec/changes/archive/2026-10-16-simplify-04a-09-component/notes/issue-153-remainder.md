# The rest of #153: one new issue and one comment

Owner ruling 2 of 2026-10-16 (`design.md`, "Owner rulings"): PR #273 closes #153, and the items outside `component`'s
scope for this PR go to the places that own that work. Task 6.2 posts these texts. Cites are on `main` at `2422d8f`,
except item 3's, which are on `main` at `e593a28`.

Item 7 of #153 was first routed as a comment on PR #272 (simplification wave PR 9b, `pkg/lifecycle`). That PR merged
on 2026-10-16 (`c6d006d`) without removing the two structs: on `main` at `e593a28`, `createOutcome` is
`pkg/lifecycle/manager.go:318` and `transitionOutcome` is `:530`, and neither name appears in `c6d006d`'s diff of
`manager.go` as removed. A comment on a merged PR reaches no one, so the item is item 3 of the new issue instead.

New issue: **#280** (filed 2026-10-16). Comment on #196: 6223455889.

## New issue

Title:
`port-refactor: rest of #153 (objectstore in the storage doc, Declaration's port lists, lifecycle one-field structs)`

Labels: `class:port-refactor`

Body:

> Three items from #153 that PR #273 (simplification wave PR 7, `component`) does not carry. The owner ruled on
> 2026-10-16 that #273 closes #153 and that these move here.
>
> ### 1. `storage/doc.go` describes `objectstore`, which is not in the tree
>
> `storage/doc.go:82` (`store, err := objectstore.NewStoreWithConfig(ctx, natsClient, config)`) and `:114-115`
> (`component := objectstore.NewComponent(config, deps)`) show a package SemEngine has not ported. A reader who
> follows the example finds no such package. Fix: describe only what `storage` offers today, and let the
> `objectstore` port (change 4) add its own example.
>
> ### 2. `component.Declaration` keeps ports and their facts in two parallel lists
>
> `component/registry.go:80-83` holds `InputPorts []Port` beside `InputFacts []PortFacts`, and the same for outputs.
> The two lists must stay index-aligned, and a reader such as the pin's message logger
> (`service/message_logger.go:347-349` at `8b99efe9`) indexes one by the other. `Port.Facts()` exists
> (`component/port_resolver.go:168`), but it returns an error, so replacing a list with calls to it changes what a
> declaration promises: today the facts are computed once at admission, and a failure refuses admission. Joining the
> lists is a design change. The two services change 3 ports (`service.ComponentManager` and the message logger) read
> these lists, so the decision belongs with that port.
>
> ### 3. `pkg/lifecycle` has two one-field structs
>
> `createOutcome` (`pkg/lifecycle/manager.go:318`, holding `Entity *graph.EntityState`) and `transitionOutcome`
> (`:530`, holding `committedRevision uint64`) each wrap one value. Each can be its plain value. Simplification wave
> PR 9b (#272, `pkg/lifecycle`) merged without them because its inventory read #153 as naming the word, not the
> package (`openspec/changes/archive/2026-10-16-simplify-04a-08-lifecycle/inventory.md:176-177`). Line numbers are on
> `main` at `e593a28`.
>
> Done when: the storage doc shows only what is in the tree; change 3's design decides whether a declaration carries
> one list of port-and-facts pairs, with its readers updated; and `createWithSource`, `createWithRegistration` and
> `transitionWithOutcome` return their plain values, with the two structs deleted.
>
> Addresses #9. Split from #153 (owner ruling 2026-10-16, #153 comment 6214880970).

## Comment for #196

> From #153 item 6, moved here on the owner's word (2026-10-16; PR #273 closes #153). `internal/lifecycleguard`
> spells one refusal several ways. A nil context gets `ErrInvalidConfig` in `Start` (`guard.go:43`) but
> `ErrInvalidData` in `Stop` (`:87`). An ended context is wrapped as invalid in `Start` (`:46`) but returned bare in
> `Stop` (`:89-90`). The refusal of a concurrent `Stop` (`:113-114`) has no test. The cleanup is passed separately to
> `Start` and `Stop`, and graph-ingest passes two different values. The fix: one refusal helper, and the cleanup given
> once. It belongs with the guard's adoption in change 3. Line numbers are from #153's filing; find the code again by
> function name.
