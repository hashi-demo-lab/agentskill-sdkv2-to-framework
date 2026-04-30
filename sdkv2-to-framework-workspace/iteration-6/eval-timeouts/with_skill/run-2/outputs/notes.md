# Migration notes — `openstack_db_database_v1`

## Scope
SDKv2 resource `resource_openstack_db_database_v1.go` ported to
`terraform-plugin-framework`, with timeouts moved to
`terraform-plugin-framework-timeouts`.

## Key migration decisions

### Timeouts: `Block`, not `Attributes`
The SDKv2 source defined `Timeouts: &schema.ResourceTimeout{Create: …, Delete: …}`,
which renders in HCL as a *block*:

```hcl
timeouts {
  create = "10m"
  delete = "10m"
}
```

To preserve that practitioner-facing syntax across the migration (see
`references/timeouts.md` "Block vs Attributes"), the framework schema uses
`timeouts.Block(...)` placed in `Blocks:` rather than `timeouts.Attributes(...)`
in `Attributes:`. Switching to attribute syntax would be a breaking HCL change
for any user with existing `timeouts { … }` blocks. Update is intentionally
*not* enabled in `Opts` because the SDKv2 resource only exposed Create + Delete
(every user attribute is `ForceNew`, so an Update timeout would be unreachable).

```go
Blocks: map[string]schema.Block{
    "timeouts": timeouts.Block(ctx, timeouts.Opts{
        Create: true,
        Delete: true,
    }),
},
```

The model carries the timeouts value with the matching tfsdk tag:

```go
type databaseDatabaseV1Model struct {
    // ...
    Timeouts timeouts.Value `tfsdk:"timeouts"`
}
```

CRUD methods read the configured timeout via the helper, applying the same
10-minute defaults the SDKv2 schema declared:

```go
createTimeout, diags := plan.Timeouts.Create(ctx, 10*time.Minute)
deleteTimeout, diags := state.Timeouts.Delete(ctx, 10*time.Minute)
```

The duration is then applied as a `context.WithTimeout`, replacing the SDKv2
pattern where `ctx` arrived already deadline-capped and timeouts were read via
`d.Timeout(schema.TimeoutCreate)`.

### `id` attribute added
SDKv2 resources implicitly carry an `id`; the framework requires it in the
schema. Added as Computed + `UseStateForUnknown` plan modifier. The ID format
is unchanged: `<instance_id>/<name>`, parsed with the existing
`parsePairedIDs` helper.

### `ForceNew` → `RequiresReplace`
All three user-facing attributes (`region`, `name`, `instance_id`) carried
`ForceNew: true` in SDKv2. Translated to
`stringplanmodifier.RequiresReplace()` plan modifiers (NOT
`RequiresReplace: true`, which is a common mis-port — see SKILL.md common
pitfalls). `region` additionally keeps `UseStateForUnknown` because it is also
`Computed`.

Because every user attribute requires replacement, `Update` is structurally
unreachable but is kept as an empty method to satisfy the `resource.Resource`
interface.

### `retry.StateChangeConf` replaced
The framework has no equivalent to SDKv2's
`helper/retry.StateChangeConf.WaitForStateContext` — and continuing to import
`helper/retry` from a migrated file fails the negative gate in
`verify_tests.sh`. Inlined a 30-line `waitForDatabaseDatabaseV1` ticker poll
matching the pattern in `references/resources.md`. The existing
`databaseDatabaseV1StateRefreshFunc` (in `db_database_v1.go`) returns
`retry.StateRefreshFunc`, which is structurally identical to
`func() (any, string, error)` and assigns through Go's type identity rules, so
that helper file was left untouched. A package-wide sweep would change its
signature for cleanliness — out of scope here.

### Import
The SDKv2 importer was passthrough
(`schema.ImportStatePassthroughContext`). Translated to
`resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)`, with a
sanity check that the imported ID contains the `instance_id/db_name`
separator. The existing `parsePairedIDs` runs in `Read`, exactly as before.

### Drift handling
SDKv2 set `d.SetId("")` when the upstream resource was gone; the framework
equivalent is `resp.State.RemoveResource(ctx)`.

### `CheckDeleted` substitute
The SDKv2 `CheckDeleted` helper unwraps 404s. In framework code it is
straightforward to inline: a `gophercloud.ErrDefault404` from
`databases.Delete` is treated as a successful delete (resource already gone).

## Test file changes

- `ProviderFactories: testAccProviders` → `ProtoV6ProviderFactories:
  testAccProtoV6ProviderFactories`. The framework provider must be served via
  `providerserver.NewProtocol6WithError` and registered alongside (or
  instead of) the SDKv2 factory in `provider_test.go`. That
  package-wide setup is outside the scope of a single-resource migration —
  the test refers to the symbol the package is expected to expose.
- Test logic itself (`testAccCheckDatabaseV1DatabaseExists`,
  `testAccCheckDatabaseV1DatabaseDestroy`, `testAccDatabaseV1DatabaseBasic`)
  is unchanged. They only consume `*Config` from `testAccProvider.Meta()` and
  the rendered HCL — neither is affected by the SDKv2→framework rewrite.
- The companion `import_openstack_db_database_v1_test.go` (not regenerated
  here) needs the same `ProviderFactories` → `ProtoV6ProviderFactories` swap.

## Provider wiring (out of file)

Per the single-release-cycle workflow, `provider.go` must:

1. Register `NewDatabaseDatabaseV1Resource` in the framework provider's
   `Resources()` method.
2. Remove the SDKv2 `resourceDatabaseDatabaseV1()` from the SDKv2
   provider's resource map.
3. Expose `testAccProtoV6ProviderFactories` for tests.

## Verification

Run from the provider repo root:

```sh
bash <skill-path>/scripts/verify_tests.sh \
  /Users/simon.lynch/git/terraform-provider-openstack \
  --migrated-files openstack/resource_openstack_db_database_v1.go \
                   openstack/resource_openstack_db_database_v1_test.go
```

The negative gate confirms neither file imports
`github.com/hashicorp/terraform-plugin-sdk/v2`.

## Pitfalls actively avoided

- `Default` is not wired into `PlanModifiers` (it is not a plan modifier in the
  framework).
- `ForceNew: true` is *not* translated to `RequiresReplace: true`; it is a
  plan modifier.
- No import of `helper/retry` survives in the migrated file.
- Schema attribute names (`region`, `name`, `instance_id`) are unchanged —
  any rename would be a state-breaking change for practitioners.
- Timeouts use `Block` (HCL block syntax), not `Attributes`, to preserve the
  user-visible config shape.
