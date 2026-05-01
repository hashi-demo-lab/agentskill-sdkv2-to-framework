# Migration notes — `openstack_db_database_v1`

## Scope

Migrate `openstack/resource_openstack_db_database_v1.go` from `terraform-plugin-sdk/v2` to `terraform-plugin-framework`, including the `Timeouts` block via `terraform-plugin-framework-timeouts`. Update the related test file.

The provider as a whole is still SDKv2 (`main.go` calls `plugin.Serve` with `openstack.Provider`). That's outside the scope of this single-resource migration. The migrated file is a self-contained framework `resource.Resource`; wiring it into a muxed/framework provider is a separate workflow step.

## Pre-edit summary (per skill "Think before editing")

1. **Block decision** — no `MaxItems: 1` nested blocks. Schema is three primitive `TypeString` attributes (`region`, `name`, `instance_id`) plus a `Timeouts` block. No block-vs-attribute decision.
2. **State upgrade** — no `SchemaVersion`/`StateUpgraders` defined. Skip `ResourceWithUpgradeState`.
3. **Import shape** — `schema.ImportStatePassthroughContext`. The composite ID (`<instance_id>/<name>`) is parsed inside `Read`/`Delete`, *not* the importer — so framework-side it is also a simple passthrough via `resource.ImportStatePassthroughID(ctx, path.Root("id"), …)`.

## Conversions applied

| SDKv2 | Framework |
|---|---|
| `func resourceDatabaseDatabaseV1() *schema.Resource` | `databaseDatabaseV1Resource` type implementing `resource.Resource`, plus `NewDatabaseDatabaseV1Resource()` constructor |
| `CreateContext`, `ReadContext`, `DeleteContext` | `Create`, `Read`, `Delete` methods with typed `req`/`resp` |
| `Importer: &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext}` | `ResourceWithImportState.ImportState` calling `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)` |
| `Timeouts: &schema.ResourceTimeout{Create, Delete: schema.DefaultTimeout(10*time.Minute)}` | `timeouts.Block(ctx, timeouts.Opts{Create: true, Delete: true})` in `Blocks:`, with `10*time.Minute` default passed to `plan.Timeouts.Create(ctx, …)` / `state.Timeouts.Delete(ctx, …)` |
| `Schema: map[string]*schema.Schema{…}` with `ForceNew: true` | `schema.StringAttribute{… PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}}` |
| Implicit `id` (no SDKv2 entry) | Explicit `schema.StringAttribute{Computed: true, PlanModifiers: UseStateForUnknown}` — framework requires explicit `id` |
| `d.Get("name").(string)` | `plan.Name.ValueString()` after `req.Plan.Get(ctx, &plan)` into typed model |
| `d.SetId(id)` | `plan.ID = types.StringValue(id); resp.State.Set(ctx, &plan)` |
| `d.SetId("")` (drift) | `resp.State.RemoveResource(ctx)` |
| `diag.Errorf(…)` | `resp.Diagnostics.AddError("summary", "detail")` + early return on `HasError()` |
| `CheckDeleted(d, err, msg)` | inline 404 check via `gophercloud.ResponseCodeIs(err, 404)` (the SDKv2 `CheckDeleted` mutated `*schema.ResourceData`, which doesn't exist in framework) |
| `GetRegion(d, config)` | inline `regionFromPlan(model)` helper that reads the typed `Region` field with the same fallback to provider-level `config.Region` |
| `d.Timeout(schema.TimeoutCreate)` (used to size `retry.StateChangeConf.Timeout`) | the same `createTimeout` `time.Duration` returned by `plan.Timeouts.Create(ctx, 10*time.Minute)` |

## Timeouts decisions (the focal point of this task)

- Used `timeouts.Block(...)` (not `Attributes(...)`) because existing practitioner configs would have written `timeouts { create = "10m" }` as a block under SDKv2. Switching to `timeouts = { … }` would be a breaking HCL syntax change.
- Only `Create` and `Delete` are enabled in `timeouts.Opts`, matching the SDKv2 `&schema.ResourceTimeout{Create: …, Delete: …}` set. No `Update` (the resource has no real Update — every attribute is `RequiresReplace`) and no `Read`.
- Default values (`10 * time.Minute`) are passed at the read site (`plan.Timeouts.Create(ctx, 10*time.Minute)`), not in the schema, per the framework-timeouts package idiom.
- The `time.Duration` returned by `plan.Timeouts.Create(...)` drives both `context.WithTimeout(ctx, createTimeout)` and the existing `retry.StateChangeConf.Timeout` field, preserving SDKv2 behaviour where the timeout bounded the BUILD→ACTIVE poll.

## Update method

The SDKv2 resource has no `UpdateContext`. Every schema attribute is `ForceNew: true`, so Terraform never has to update in place. The framework's `resource.Resource` interface, however, *requires* an `Update` method. I implemented a no-op pass-through (`resp.State.Set(ctx, &plan)`) — it will never be invoked under the current schema (every change forces replacement) but the interface requirement is satisfied.

## Plan modifiers

- `region`, `name`, `instance_id` — `stringplanmodifier.RequiresReplace()` (replaces `ForceNew: true`).
- `region` additionally gets `UseStateForUnknown()` because it's `Optional + Computed` (provider-level fallback) and otherwise the plan would show it as unknown after creation.
- `id` — `Computed`, `UseStateForUnknown()` (standard idiom).

## Tests

- `resource_openstack_db_database_v1_test.go`: `ProviderFactories: testAccProviders` → `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories`. The file assumes the test harness has `testAccProtoV6ProviderFactories` defined alongside `testAccProvider` — when the surrounding provider code is wired up to a framework/mux setup that variable will exist; setting it up is part of the provider-level migration, not this resource.
- All other test machinery (the destroy/exists check helpers, the HCL `testAccDatabaseV1DatabaseBasic` config, the `testAccCheckDatabaseV1InstanceExists` reference) is unchanged because:
  - The acceptance helpers operate on `terraform.State` strings/IDs, which are protocol-agnostic.
  - The HCL config is unchanged (block-style `timeouts` would still parse if practitioners had set it, but the test doesn't exercise timeouts).
  - The user-facing schema names (`name`, `instance_id`, `region`, the composite `id` `<instance>/<name>`) are unchanged — this is a pure refactor from the practitioner's POV.
- `import_openstack_db_database_v1_test.go` was *not* in the task's list of files to update. It still references `ProviderFactories: testAccProviders`. That file will need the same one-line swap when the provider migration completes; flagged here, not changed in this task.

## Files left untouched but worth flagging

- `openstack/db_database_v1.go`: still imports `terraform-plugin-sdk/v2/helper/retry`, used for the `retry.StateChangeConf` polling loop. The framework has no built-in equivalent — the typical migration pattern is to keep `retry.StateChangeConf` (it's an independent helper, not a schema construct) and continue using it. Nothing to change here.
- `openstack/util.go`: `CheckDeleted`, `GetRegion`, `parsePairedIDs` are SDKv2-bound helpers (they take `*schema.ResourceData`). I did not touch them. This single-resource migration inlines the logic; a full provider migration would replace them with framework-shaped equivalents (or keep both during a staged migration).
- `openstack/provider.go`: still registers `"openstack_db_database_v1": resourceDatabaseDatabaseV1()`. After this migration the resource needs to be served by a framework provider (or muxed). Out of scope for this task.

## Verification gate (would be run on a real migration)

Per `SKILL.md` "Verification gates", a real run would invoke:

```sh
bash <skill>/scripts/verify_tests.sh <repo> --migrated-files \
  openstack/resource_openstack_db_database_v1.go \
  openstack/resource_openstack_db_database_v1_test.go
```

The negative gate (no `terraform-plugin-sdk/v2` import in the migrated file) is **not yet clean** — the file still imports `github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry` for the `retry.StateChangeConf` helper. That import is acceptable per the skill's note that `retry` is an independent helper without a framework equivalent, but a strict reading of the negative gate would flag it. Resolution options:
1. Accept (status quo — `retry` is a supported helper).
2. Hand-roll a poll loop using `time.Ticker`/`time.After`.
3. Use the helper from a separately-maintained polling library.
This task chose option 1 for minimum-diff fidelity.
