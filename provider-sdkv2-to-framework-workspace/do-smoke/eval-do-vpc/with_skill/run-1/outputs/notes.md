# Migration notes: digitalocean_vpc

## Scope and assumptions

- Single-resource migration: only `resource_vpc.go` and `resource_vpc_test.go` were touched. The rest of the provider (and the `acceptance` package) remains SDKv2.
- The skill's single-release-cycle workflow assumes the *whole* provider is being migrated in one pass. The DigitalOcean repo is far from that state, so this delivery is the per-resource artefact only — wiring it into a working build needs follow-up that's explicitly out of scope here.

## Per-attribute mapping

| Old (SDKv2) | New (framework) | Notes |
|---|---|---|
| `Type: TypeString, Required: true, ValidateFunc: validation.NoZeroValues` (name) | `schema.StringAttribute{Required: true, Validators: []validator.String{nonZeroStringValidator{}}}` | Local validator — see "Validators not in the validators package" below. |
| `Type: TypeString, Required: true, ForceNew: true, ValidateFunc: validation.NoZeroValues` (region) | `StringAttribute{Required: true, PlanModifiers: [stringplanmodifier.RequiresReplace()], Validators: [nonZeroStringValidator{}]}` | `ForceNew → RequiresReplace` plan-modifier. |
| `Type: TypeString, Optional: true, ValidateFunc: StringLenBetween(0, 255)` (description) | `StringAttribute{Optional: true, Validators: [stringvalidator.LengthBetween(0, 255)]}` | Direct validators-package mapping. |
| `Type: TypeString, Optional: true, Computed: true, ForceNew: true, ValidateFunc: IsCIDR` (ip_range) | `StringAttribute{Optional: true, Computed: true, PlanModifiers: [RequiresReplace(), UseStateForUnknown()], Validators: [cidrValidator{}]}` | The skill recommends `cidrtypes.IPv4Prefix` from `terraform-plugin-framework-nettypes` for CIDR. I kept a plain `types.String` plus a small custom validator: switching to `cidrtypes` would change the attribute *type* (model field becomes `cidrtypes.IPv4Prefix`) which is a heavier change than this resource needs. Flagging here. |
| `Computed: true` (urn, default, created_at) | `Computed: true` on each | No change. |
| `Timeouts: &schema.ResourceTimeout{Delete: schema.DefaultTimeout(2 * time.Minute)}` | `Blocks: { "timeouts": timeouts.Block(ctx, timeouts.Opts{Delete: true}) }` and `state.Timeouts.Delete(ctx, 2*time.Minute)` in `Delete` | Used `timeouts.Block` (not `Attributes`) to preserve SDKv2 block syntax; default of 2m is set in the call site, not the schema. |
| `Importer: &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext}` | `ImportState` implementing `resource.ResourceWithImportState`, calls `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)` | Standard passthrough. |

## CRUD method shape

- All four CRUD methods follow the skill's worked example: read plan/state into a typed `vpcResourceModel`, gate on `resp.Diagnostics.HasError()`, write back via `resp.State.Set(ctx, model)`.
- Drift handling in `Read`: 404 from the API → `resp.State.RemoveResource(ctx)` (SDKv2 was `d.SetId("")`).
- `Update` mirrors the SDKv2 `d.HasChanges("name", "description")` gate: skip the API call when neither field changed. Preserved the original quirk where the request body's `Default` field is sourced from prior state's computed `default` attribute (not user input) — it has to be read from `req.State`, not `req.Plan`, because it's `Computed`-only.
- `Delete` replaces `retry.RetryContext` with the in-file ticker pattern from `references/resources.md` ("Replacing `retry.StateChangeConf`"). Same retry semantics: only retries on 403 / 409, fails fast on anything else.
- `Configure` casts `req.ProviderData` to `*config.CombinedConfig`; if the provider configure step changes shape during the eventual provider migration this assertion will need to be updated.

## Validators not in the validators package

- `validation.NoZeroValues`: there is no direct port in `terraform-plugin-framework-validators`. Wrote a small `nonZeroStringValidator` in-file. (`stringvalidator.LengthAtLeast(1)` is *almost* the same but rejects null + empty differently from NoZeroValues; a faithful port is cheap.)
- `validation.IsCIDR`: skill points to `cidrtypes.IPv4Prefix` from `terraform-plugin-framework-nettypes`. I used a hand-rolled validator instead because adopting the custom type changes the model field type and downstream comparisons. Worth re-evaluating during the provider-wide pass.

## Tests

- Switched `ProviderFactories` → `ProtoV6ProviderFactories` per the skill's TDD-gate guidance.
- Replaced `terraform-plugin-sdk/v2/helper/resource` → `terraform-plugin-testing/helper/resource`; same for `terraform.State`.
- Introduced a reference to `acceptance.TestAccProtoV6ProviderFactories` that does not yet exist in the repo's `acceptance` package — populating it is part of the wider provider migration. A note in the test file flags this.
- Pulled `TestAccDigitalOceanVPC_importBasic` into the migrated test file alongside the basic CRUD tests so the import-state-verify step can be exercised together.

## Things the skill's references didn't cover well

1. **`mutexkv` and other helper packages**: the resource uses an internal `mutexkv` package to serialise creates per region. The skill has nothing to say about non-terraform-SDK helpers; in practice they pass through unchanged but it would be nice to have a one-liner reminder ("private utility imports are typically left alone").
2. **Computed-but-API-meaningful fields on update**: the `Default` field is `Computed` in the schema yet its prior-state value is included in the update request body. There's no obvious skill guidance for "computed field that the API still needs in the update body" — I sourced from `req.State` rather than `req.Plan`. Worth a sentence in `references/state-and-types.md`.
3. **Partial-migration testing**: the skill explicitly excludes mux, but a "single-resource walkthrough on an otherwise-SDKv2 provider" hits the same wall — the test file can't actually run without the provider half being migrated, or without mux. The skill should call this out more bluntly: a per-resource migration is not independently runnable, full stop.
4. **`validation.NoZeroValues`**: the validators table doesn't list it. It maps poorly to `LengthAtLeast(1)` (different null semantics), so a small bespoke validator is the right answer; spelling that out would save the migrator a minute.
5. **`validation.IsCIDR` migration tradeoff**: the table points at `cidrtypes.IPv4Prefix`, which is correct for "API may renormalise" cases but heavier than needed for "must parse as CIDR". A two-line note distinguishing the two would be helpful.

## Verification (would-be steps)

The task explicitly forbids running `go build/vet/test/mod tidy` against the read-only clone, and the migrated files live outside the clone. So none of the skill's verification gates (`scripts/verify_tests.sh`) were run. To verify in a real migration:
- Place `migrated/resource_vpc.go` over `digitalocean/vpc/resource_vpc.go` (and the test file likewise).
- Stand up `acceptance.TestAccProtoV6ProviderFactories` either by completing the whole-provider migration or by a temporary mux harness.
- Run `bash <skill-path>/scripts/verify_tests.sh <repo> --migrated-files digitalocean/vpc/resource_vpc.go digitalocean/vpc/resource_vpc_test.go` and confirm the negative gate passes (no `terraform-plugin-sdk/v2` import in either file).
