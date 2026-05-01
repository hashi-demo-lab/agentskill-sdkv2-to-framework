# openstack_db_user_v1 — SDKv2 → Framework migration

## Scope

Single-resource migration of `openstack_db_user_v1` from
`terraform-plugin-sdk/v2` to `terraform-plugin-framework`, with a major-version
upgrade of the `password` field from `Sensitive` to `Sensitive + WriteOnly`.

## Schema decisions

| Attribute     | Old (SDKv2)                                      | New (framework)                                                                          |
|---------------|--------------------------------------------------|------------------------------------------------------------------------------------------|
| `id`          | (implicit)                                       | `StringAttribute{Computed, UseStateForUnknown}`                                          |
| `region`      | `Optional+Computed+ForceNew`                     | `StringAttribute{Optional, Computed, RequiresReplace, UseStateForUnknown}`               |
| `name`        | `Required+ForceNew`                              | `StringAttribute{Required, RequiresReplace}`                                             |
| `instance_id` | `Required+ForceNew`                              | `StringAttribute{Required, RequiresReplace}`                                             |
| `password`    | `Required+ForceNew+Sensitive`                    | `StringAttribute{Required, Sensitive, WriteOnly, RequiresReplace}` — **NOT Computed**    |
| `host`        | `Optional+ForceNew`                              | `StringAttribute{Optional, RequiresReplace}`                                             |
| `databases`   | `TypeSet (Optional+Computed) of TypeString`      | `SetAttribute{ElementType: StringType, Optional, Computed, UseStateForUnknown}`          |
| `timeouts`    | `Timeouts{Create, Delete}`                       | `timeouts.Block(ctx, Opts{Create: true, Delete: true})`                                  |

## Key write-only handling

- `password` has `Sensitive: true` AND `WriteOnly: true` — `Computed` removed
  (the framework rejects `WriteOnly + Computed` at provider boot).
- In `Create`, password is read from `req.Config` (it is null in `req.Plan` for
  write-only attributes). Plan/state writes set `Password = types.StringNull()`
  to ensure nothing is persisted to state.
- Test step adds an `ImportState`/`ImportStateVerify` round-trip with
  `ImportStateVerifyIgnore: []string{"password"}`, since the imported state has
  a null password by design and would otherwise fail comparison.

## Other migration notes

- Replaced `helper/retry.StateChangeConf.WaitForStateContext` with an inline
  `waitForDatabaseUserV1Active` poll using `slices.Contains` and
  `time.NewTicker`, so the migrated file imports no SDKv2 packages.
- `Set: schema.HashString` dropped — framework `SetAttribute` handles
  uniqueness internally.
- `Update` is a no-op stub: every user-facing attribute carries
  `RequiresReplace`, so any change forces recreation (matching the SDKv2
  `ForceNew: true` behaviour).
- `Delete` reads from `req.State` (not `req.Plan`, which is null on Delete).
- 404 on delete is swallowed (mirrors SDKv2 `CheckDeleted`).
- Test factory flipped from `ProviderFactories: testAccProviders` to
  `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories` per the skill's
  testing guidance and pitfall list.

## Output files

- `migrated/resource_openstack_db_user_v1.go`
- `migrated/resource_openstack_db_user_v1_test.go`
- `agent_summary.md`
