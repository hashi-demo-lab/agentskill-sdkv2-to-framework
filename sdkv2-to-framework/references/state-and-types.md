# State, plan, and config access — typed values

## Quick summary
- SDKv2 used `*schema.ResourceData` with `d.Get("path")` and `interface{}` casting; the framework uses typed model structs and `(req|resp).{Plan|State|Config}.Get(ctx, &model)`.
- Every attribute corresponds to a typed field on the model: `types.String`, `types.Int64`, `types.Bool`, `types.List`, `types.Set`, `types.Map`, `types.Object`.
- Field names on the model use `tfsdk:"attribute_name"` struct tags to map to schema attribute names.
- The `types.*` values can be null, unknown, or known — *always* check `IsNull()`/`IsUnknown()` before calling `ValueString()`/`ValueInt64()`/etc., or you get the type's zero value.
- Custom types implement `attr.Type`/`basetypes.StringTypable` — useful for normalisation (replaces some `DiffSuppressFunc`/`StateFunc` uses).

## The model struct pattern

For every resource/data source, define a struct whose fields map to schema attributes:

```go
type thingModel struct {
    ID        types.String `tfsdk:"id"`
    Name      types.String `tfsdk:"name"`
    Tags      types.Map    `tfsdk:"tags"`
    Endpoint  types.Object `tfsdk:"endpoint"` // single nested attribute
    Rules     types.List   `tfsdk:"rules"`    // list of nested
    CreatedAt types.String `tfsdk:"created_at"`
}
```

For nested attributes you can also use a typed nested struct (cleaner):

```go
type endpointModel struct {
    URL  types.String `tfsdk:"url"`
    Port types.Int64  `tfsdk:"port"`
}
type thingModel struct {
    ID       types.String   `tfsdk:"id"`
    Endpoint *endpointModel `tfsdk:"endpoint"`
}
```

Pointer-to-struct for `SingleNestedAttribute` means "nil when null". For lists/sets of nested objects, use `[]endpointModel` (the framework converts to/from `types.List` automatically when reading via `Plan.Get`).

## Reading from plan / state / config

```go
var plan thingModel
resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
if resp.Diagnostics.HasError() { return }

name := plan.Name.ValueString()
```

`req.Plan` is what Terraform intends to apply. `req.State` is the prior state. `req.Config` is the raw config (with unknown values where computed will be set). Pick by use site:

| Method | Available |
|---|---|
| `Create` | `req.Plan`, `req.Config`, *not* `req.State` (no prior state) |
| `Read` | `req.State` only |
| `Update` | `req.Plan`, `req.State`, `req.Config` |
| `Delete` | `req.State` only |
| Data source `Read` | `req.Config` only |
| `ModifyPlan` | `req.Plan`, `req.State`, `req.Config`, plus `resp.Plan` to write modifications |

## Writing back to state

```go
plan.ID = types.StringValue(id)
plan.CreatedAt = types.StringValue(now.Format(time.RFC3339))
resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
```

`resp.State.Set` writes the entire model. There's also `resp.State.SetAttribute(ctx, path, value)` for partial updates.

## Null / unknown / known

`types.String` (and the others) is a tri-state value: null, unknown, or known.

```go
var s types.String

s.IsNull()    // true if Terraform has no value (Optional + not set)
s.IsUnknown() // true if value is yet to be computed (Computed during plan)
// known iff !IsNull() && !IsUnknown()

s.ValueString() // "" if null/unknown; the actual string if known
```

`ValueString()` returning empty for null is a frequent bug source. Check `IsNull()` first when distinguishing "user explicitly set empty string" from "user didn't set it".

## Constructing typed values

```go
types.StringValue("hello")   // known string
types.StringNull()           // null string
types.StringUnknown()        // unknown string
types.Int64Value(42)
types.BoolValue(true)
types.Float64Value(1.5)
```

Pointer helpers:
```go
types.StringPointerValue(stringPtr)  // null if pointer is nil
```

## Lists, sets, maps

For homogeneous element types, the framework provides:

```go
list, diags := types.ListValueFrom(ctx, types.StringType, []string{"a","b","c"})
resp.Diagnostics.Append(diags...)
```

Or, on a model field, just use `[]string` and let `Plan.Get` convert:

```go
type model struct {
    Tags []string `tfsdk:"tags"`
}
```

## `basetypes` and custom types

The `types` package is convenience aliases over `basetypes`. For custom types (normalised JSON, lowercase strings, etc.), implement `basetypes.StringTypable` (or `Int64Typable`, etc.) and a custom value type embedding `basetypes.StringValue`.

This is the right migration path for many SDKv2 `DiffSuppressFunc`/`StateFunc` uses — the framework compares values via the type's `Equal` method, so a normalising custom type makes equivalent representations compare equal automatically.

A minimal example:
```go
type lowerStringType struct{ basetypes.StringType }

func (t lowerStringType) ValueFromString(ctx context.Context, in basetypes.StringValue) (basetypes.StringValuable, diag.Diagnostics) {
    return lowerStringValue{StringValue: basetypes.NewStringValue(strings.ToLower(in.ValueString()))}, nil
}

type lowerStringValue struct{ basetypes.StringValue }
```

Wire onto the schema attribute via the `CustomType` field:
```go
"name": schema.StringAttribute{
    Required:   true,
    CustomType: lowerStringType{},
}
```

Use sparingly — the type machinery is cognitive overhead. For one-off normalisation, a plan modifier or `ModifyPlan` may be simpler.

### Off-the-shelf custom types

Before writing a custom type by hand, check whether HashiCorp's companion packages already provide what you need. The two most common SDKv2 `DiffSuppressFunc` cases have ready answers:

- **`github.com/hashicorp/terraform-plugin-framework-jsontypes`** — `jsontypes.Normalized` (whitespace/key-order-insensitive JSON comparison) and `jsontypes.Exact` (preserve formatting). Use for any attribute holding JSON: API request bodies, OpenAPI specs, IAM policy documents.
- **`github.com/hashicorp/terraform-plugin-framework-nettypes`** — `cidrtypes.IPv4Prefix`/`IPv6Prefix`, `iptypes.IPv4Address`/`IPv6Address`, MAC address types via `hwtypes`. Use for any CIDR / IP / MAC attribute where the API may rewrite the value (e.g., `192.168.1.0/24` vs `192.168.1.0/24`, or `::1` vs `0:0:0:0:0:0:0:1`).

If neither covers your case, write a custom type. JSON normalisation and CIDR handling alone account for most real-world `DiffSuppressFunc` migrations.

### Destructive `StateFunc` — do NOT use a destructive custom type

A common SDKv2 pattern is `StateFunc: hashString` on a secret attribute (the raw secret never persists in state; only its hash does). It's tempting to translate this directly to a custom type whose `ValueFromString` hashes the input. **Don't.** `CustomType` is wired at the schema level, so `req.Config`, `req.Plan`, and `req.State` all decode through `ValueFromString` — there is no `req.Plan` value with the unhashed original. By the time `Create` reads the plan, the secret is already hashed; the API call sends a hash and silently fails.

Three correct patterns, in preference order:

1. **`WriteOnly: true`** (framework v1.14+). The raw value flows from config to your API call but is never persisted. This is the framework's purpose-built answer for "secret Terraform doesn't need to read back" — see `references/sensitive-and-writeonly.md`. If WriteOnly fits your case, prefer it.
2. **Plain `types.String` + hash in `Create`/`Update`, then store the hash via `resp.State.SetAttribute`**. The schema attribute carries no `CustomType`; the resource model holds raw values; you hash explicitly before writing state. Drift detection becomes the resource's responsibility (compare hashes in `Read` against the stored hash and re-issue when they differ).
3. **Non-destructive custom type that preserves raw + exposes hash via `Equal`**. The value type holds both the raw input AND a derived hash; `ValueFromString` keeps the raw, `Equal` compares hashes. This is the closest analogue to SDKv2's `DiffSuppressFunc`-via-hash but is genuinely fiddly. Reach for it only when (1) and (2) don't apply.

If you have already shipped a destructive custom type, it's a real production bug — practitioners' API calls send the hash to the upstream API. Treat it as a state-breaking fix.

## SDKv2 → framework cheatsheet

| SDKv2 | Framework |
|---|---|
| `d.Get("name").(string)` | `var m model; req.Plan.Get(ctx, &m); m.Name.ValueString()` |
| `d.Get("tags").(map[string]interface{})` | `m.Tags` (typed `types.Map` or `map[string]string`) |
| `d.Set("name", "foo")` | `m.Name = types.StringValue("foo"); resp.State.Set(ctx, m)` |
| `d.Id()` | `state.ID.ValueString()` |
| `d.SetId(id)` | `m.ID = types.StringValue(id); resp.State.Set(ctx, m)` |
| `d.SetId("")` | `resp.State.RemoveResource(ctx)` |
| `d.HasChange("x")` | `!plan.X.Equal(state.X)` |
| `d.GetOk("x")` | `if !m.X.IsNull() && !m.X.IsUnknown() { ... }` |
| `d.GetOkExists("x")` | `!m.X.IsNull()` — the framework distinguishes "null" (not set) from "known zero" (explicitly set to `""`/`0`/`false`), so `!IsNull()` is the right test |
| `d.Partial(true)` / `d.SetPartial(...)` | gone — partial state is handled by writing each field to `resp.State` as it succeeds |
