# Migration notes — `openstack_lb_pool_v2` schema

Schema-only port. CRUD methods, the model struct, `Configure`, `Metadata`,
`ImportState`, and tests are out of scope for this eval (per the task brief)
and are not migrated. The notes below cover decisions that affect the
schema *shape* and a few caveats the next person should know about before
finishing the resource end-to-end.

## Field-by-field translation summary

| SDKv2 attribute | Framework shape | Notes |
|---|---|---|
| `region` (Optional, Computed, ForceNew) | `StringAttribute` + `RequiresReplace` + `UseStateForUnknown` | Default plan-modifier pair for an Optional+Computed+ForceNew field. |
| `tenant_id` (Optional, Computed, ForceNew) | `StringAttribute` + `RequiresReplace` + `UseStateForUnknown` | Same shape as `region`. |
| `name`, `description` | `StringAttribute{Optional: true}` | Plain. |
| `protocol` (Required, ForceNew, StringInSlice) | `StringAttribute` + `RequiresReplace` + `stringvalidator.OneOf` | |
| `loadbalancer_id`, `listener_id` (Optional, ForceNew, ExactlyOneOf) | `StringAttribute` + `RequiresReplace` + `stringvalidator.ExactlyOneOf` | The cross-attribute constraint moves from a schema field to a per-attribute validator referenced via `path.MatchRoot`. |
| `lb_method` (Required, StringInSlice) | `StringAttribute` + `stringvalidator.OneOf` | |
| `persistence` (TypeList, MaxItems:1, nested) | `ListNestedBlock` + `listvalidator.SizeAtMost(1)` | See `reasoning.md`. |
| `alpn_protocols` (TypeSet of strings, Optional+Computed, per-element StringInSlice) | `SetAttribute{ElementType: types.StringType}` + `setvalidator.ValueStringsAre(stringvalidator.OneOf(...))` | The per-element validator is wrapped via `setvalidator.ValueStringsAre`. |
| `ca_tls_container_ref`, `crl_container_ref`, `tls_container_ref` | `StringAttribute{Optional: true}` | Plain. |
| `tls_enabled` | `BoolAttribute{Optional: true}` | |
| `tls_ciphers` (Optional, Computed) | `StringAttribute{Optional: true, Computed: true}` | "unsetting results in a default value" comment preserved. |
| `tls_versions` (TypeSet, Optional+Computed, per-element StringInSlice) | `SetAttribute` + `setvalidator.ValueStringsAre(stringvalidator.OneOf(...))` | Same shape as `alpn_protocols`. |
| `admin_state_up` (TypeBool, Default: true, Optional) | `BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true)}` | `Default` requires `Computed: true` in the framework. |
| `tags` (TypeSet of strings, Set: schema.HashString) | `SetAttribute{ElementType: types.StringType}` | `Set: schema.HashString` is dropped — framework `SetAttribute` handles uniqueness internally. |
| (implicit) `id` | `StringAttribute{Computed, UseStateForUnknown}` | SDKv2 gave you `id` for free; framework requires it to be declared explicitly. |

## Caveats / things the next person needs to handle

1. **`id` was implicit in SDKv2; explicit in the framework.** The migrated
   schema declares `id` as `Computed` with `UseStateForUnknown` so plan
   output stays quiet between applies. This is required — without it the
   resource would not have an `id` field at the framework level.

2. **Cross-attribute validators are placed on *both* `loadbalancer_id` and
   `listener_id`.** Strictly only one is needed (the validator runs on either
   attribute), but symmetric placement makes error attribution clearer and
   matches the SDKv2 idiom of `ExactlyOneOf` on both. This matches the
   guidance in `references/validators.md`.

3. **`persistence` block decision is judgment-bearing.** See `reasoning.md`.
   On the *next* provider major bump, revisit and convert to
   `SingleNestedAttribute` for cleaner per-field plan modifiers / validators.

4. **`alpn_protocols` and `tls_versions` use `setvalidator.ValueStringsAre`
   to apply per-element string validation.** SDKv2's nested
   `Elem: &schema.Schema{ValidateFunc: ...}` shape doesn't have a 1:1
   framework analogue; `ValueStringsAre` from `setvalidator` is the right
   wrapper for "every element must satisfy this string validator".

5. **`Set: schema.HashString` was dropped from `tags`.** `SetAttribute`
   computes uniqueness internally. There is no `Set:` field to translate;
   leaving in any reference to `schema.HashString` would be a leftover
   SDKv2 import.

6. **Timeouts (10 min Create/Update/Delete) are not in this schema.**
   Schema-only scope by task definition, but for the full migration: add a
   `timeouts.Attributes(ctx, timeouts.Opts{Create: true, Update: true,
   Delete: true})` to `Attributes` and use the
   `terraform-plugin-framework-timeouts` package — see
   `references/timeouts.md`. The retry loops in the SDKv2 CRUD methods
   consume `d.Timeout(schema.TimeoutCreate)`, which becomes the
   `timeouts.Create(ctx, defaultTimeout)` API.

7. **Importer is currently a custom function (not passthrough).** The SDKv2
   `resourcePoolV2Import` does extra work — calls the API, walks
   `pool.Listeners[0].ID` / `pool.Loadbalancers[0].ID`, and sets one of
   `listener_id` or `loadbalancer_id`. In the framework this becomes
   `ImportState` on the resource type and *cannot* be a plain
   `resource.ImportStatePassthroughID`. See `references/import.md`. Out of
   scope for this eval but flagged for the full migration.

8. **No `SchemaVersion`/`StateUpgraders` on this resource.** The SDKv2
   schema has no `SchemaVersion` field or `StateUpgraders`, so no state
   upgrade concerns. Confirmed by inspection of the source file.

9. **`alpn_protocols` and `tls_versions` are Optional + Computed** because
   "unsetting this parameter results in a default value" (per the source
   comment). The framework treats Optional+Computed correctly — the API
   may return a different value than the user wrote, and the framework
   will use whatever ends up in state. No `Default` in the SDKv2 source
   for these (the API supplies it), so no `Default` in the framework
   either.

