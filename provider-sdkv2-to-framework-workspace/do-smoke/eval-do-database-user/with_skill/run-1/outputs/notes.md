# Migration notes — `digitalocean_database_user`

## Sensitive vs WriteOnly decision for `password`

**Decision: `Sensitive: true` only — NOT `WriteOnly`.** The same applies to
`access_cert` and `access_key` (Kafka users).

### Decision rule (verbatim from `references/sensitive-and-writeonly.md`)

> **Which one do I want?** Ask: *does Terraform need to read this value back
> later?* (drift detection, cross-resource references, import-verify, etc.)
> If **yes** → `Sensitive: true` only; the value lives in state, redacted
> from output. If **no** (one-time creds, initial passwords, rotation seeds)
> → `Sensitive: true` AND `WriteOnly: true`; the value never persists. The
> default for migrations is **Sensitive only** — flipping to `WriteOnly` is
> a practitioner-visible breaking change.

### Applying the rule to this resource

The task statement makes the answer unambiguous:

- The DigitalOcean API **returns** the password on resource creation; the
  practitioner does not supply it. The SDKv2 schema reflected this with
  `Computed: true, Sensitive: true` (no `Required` / `Optional`).
- Terraform stores the password in state so **downstream resources can
  reference it** — e.g., feeding the value into a Kubernetes Secret, an
  application config, or another resource's connection string. That is the
  exact "needs to read it back later" case the rule identifies as
  `Sensitive` only.
- Removing the value from state would break those references and silently
  corrupt downstream resources that read `digitalocean_database_user.x.password`.

So: **answer to "does Terraform need to read this value back later?" is
YES → Sensitive only.**

### Independent disqualifier: WriteOnly + Computed cannot coexist

Even setting aside the cross-resource-reference question, the
`sensitive-and-writeonly.md` "Hard rules" section makes this unambiguous:

> **`WriteOnly` and `Computed` cannot coexist on the same attribute.** A
> write-only value isn't persisted; making it computed would need the
> framework to materialise a value in state, which contradicts write-only's
> whole point. The framework rejects this at boot for top-level scalars.

`password` here is intrinsically `Computed` (the API generates it; the
practitioner cannot supply it). Marking it `WriteOnly` would be rejected
by `ValidateImplementation` at provider boot. So the rule is doubly
binding: (1) we need the value in state, and (2) the framework forbids
the combination anyway.

### When WriteOnly *would* have been right

A different shape — e.g., a `password` attribute that the practitioner
supplied as `Required` to set the initial password and that Terraform
never needed to round-trip — would be the canonical `WriteOnly`
candidate. That is not this resource. The DO API generates the password
server-side and the value participates in cross-resource graph wiring.

### Practitioner-test breaking-change consideration

The reference file also flags that switching an existing `Sensitive` to
`WriteOnly` is a breaking change for tests that assert the value via
`TestCheckResourceAttrSet` / `TestCheckResourceAttr`. The migrated test
file in this output directory keeps those `TestCheckResourceAttrSet`
assertions intact precisely because we kept the field as Sensitive. If a
future major-version bump introduced a separate practitioner-supplied
`initial_password` field, that one would be a WriteOnly candidate; this
field is not.

## Other migration choices made

- **Schema split**: `name`, `cluster_id`, `mysql_auth_plugin`, `role`,
  `password`, `access_cert`, `access_key`, plus the synthesised `id`, are
  attributes. `settings` is a `ListNestedBlock` (with nested `acl` and
  `opensearch_acl` blocks) to preserve the practitioner-visible
  `settings { acl { ... } }` HCL syntax — see `references/blocks.md`
  decision rule for `TypeList of *schema.Resource`.
- **`ForceNew: true`** on `name` and `cluster_id` →
  `stringplanmodifier.RequiresReplace()`.
- **`validation.NoZeroValues`** → `stringvalidator.LengthAtLeast(1)` (the
  documented mapping for non-empty strings; `NoZeroValues` is not a
  literal one-to-one but the semantic match for `TypeString`).
- **`validation.StringInSlice(...)`** → `stringvalidator.OneOf(...)`.
- **`DiffSuppressFunc`** on `mysql_auth_plugin` → custom plan modifier
  `mysqlAuthPluginDefaultModifier` that mirrors the original logic
  (`old == caching_sha2_password && new == ""` → no diff).
- **`Importer.State`** parsing `id,name` composite → `ImportState` method
  that splits the import ID and writes `id`, `cluster_id`, `name` via
  `resp.State.SetAttribute` (per `references/import.md`).
- **`mutexKV`** / `applyDatabaseUserAttributes` / `makeDatabaseUserID` /
  permission-normalisers retained verbatim (no SDKv2 dependencies in
  them).
- **CRUD signatures** rewritten to framework's `(ctx, req, resp)` shape;
  diagnostics replace `diag.Errorf`.
- **Computed scalars** (`id`, `role`, `password`, `access_cert`,
  `access_key`) carry `stringplanmodifier.UseStateForUnknown()` to avoid
  spurious `(known after apply)` re-derivation on subsequent plans.
- **Test file**: `ProviderFactories` → `ProtoV6ProviderFactories`,
  `terraform-plugin-sdk/v2/helper/resource` → `terraform-plugin-testing/helper/resource`,
  `terraform-plugin-sdk/v2/terraform` → `terraform-plugin-testing/terraform`.
  The test references `acceptance.TestAccProtoV6ProviderFactories`, which
  the broader provider migration will need to expose alongside the
  existing `TestAccProviderFactories`. No assertions on `password` were
  removed (see Sensitive-vs-WriteOnly section above).
- The provider definition itself is still SDKv2 in this repo. Per the
  task's "Don't migrate anything else in the repo" instruction, the
  output here is the per-resource framework code only; integrating it
  would require either a `terraform-plugin-mux` setup or a
  full-provider migration to publish the resource via
  `providerserver.NewProtocol6WithError` — both out of scope for this
  task.
