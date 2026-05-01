# Resource import

## Quick summary
- SDKv2 `Importer: &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext}` becomes the resource type implementing `resource.ResourceWithImportState`.
- The simplest case (ID is the primary identifier): use `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)`.
- Custom `Importer.StateContext` parsing composite IDs (`region/resource_id`) becomes manual `path.Root` setting in `ImportState`.
- The import method runs *before* `Read`, so you can't fetch from the API yet; just parse the ID into state fields and let `Read` populate the rest.
- Multi-resource imports (one import call seeding multiple resources) are out of scope — the framework supports it but it's rare; refer to HashiCorp docs if you need it.

## Old shape (SDKv2) — passthrough

```go
&schema.Resource{
    Importer: &schema.ResourceImporter{
        StateContext: schema.ImportStatePassthroughContext,
    },
}
```

## New shape (framework) — passthrough

```go
var _ resource.ResourceWithImportState = &thingResource{}

func (r *thingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
    resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
```

`ImportStatePassthroughID` reads `req.ID` (whatever `terraform import myprov_thing FOO` was given) and writes it to the path you specify. If the practitioner used the modern `import { identity = {...} }` block instead of a string ID, `req.ID` is empty and the helper writes nothing to state — the attribute ends up null. The framework passes `req.Identity` through to `resp.Identity` automatically; `Read` can then look up the resource via identity. So `ImportStatePassthroughID` works for both legacy and identity-block imports out of the box, with no extra wiring on simple single-attribute imports. After import, `Read` runs to populate the rest of the state.

## Composite IDs

If the SDKv2 importer parsed a composite ID like `region/resource_id`:

```go
// SDKv2
StateContext: func(ctx context.Context, d *schema.ResourceData, m interface{}) ([]*schema.ResourceData, error) {
    parts := strings.SplitN(d.Id(), "/", 2)
    if len(parts) != 2 {
        return nil, fmt.Errorf("expected region/id, got %q", d.Id())
    }
    d.Set("region", parts[0])
    d.SetId(parts[1])
    return []*schema.ResourceData{d}, nil
},
```

Becomes:

```go
func (r *thingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
    parts := strings.SplitN(req.ID, "/", 2)
    if len(parts) != 2 {
        resp.Diagnostics.AddError(
            "invalid import ID",
            fmt.Sprintf("expected 'region/id', got %q", req.ID),
        )
        return
    }
    resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("region"), parts[0])...)
    resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}
```

`resp.State.SetAttribute` writes a single attribute by path; the framework fills in the rest as null/unknown until `Read` runs.

## Why ImportState runs before Read

`ImportState` only knows the import ID string. It cannot call the API. Its job is to write whatever the API client will need to *find* the resource — typically just the resource's ID, sometimes a region or namespace.

After `ImportState` returns, the framework calls `Read` with the partial state. `Read` then makes the API call and populates the rest.

This is why parsing a composite ID into `region` + `id` is fine in `ImportState`, but trying to *validate* that the resource exists in that region is wrong — leave that to `Read`.

## What ImportState should NOT do

- Don't call the API client from `ImportState`. The design separates parsing (translate the import argument into state) from fetching (`Read` does the API call afterward). Putting an API call in `ImportState` would make import fragile if the API is briefly unavailable.
- Don't try to populate computed-only fields. Set just enough that `Read` can find the resource. `Read` populates the rest.
- Don't add validation for "is the ID well-formed beyond what's required to dispatch?" — that, again, belongs in `Read` if needed.

## Tests

The standard `ImportStateVerify` test step works:

```go
resource.TestStep{
    ResourceName:      "myprov_thing.test",
    ImportState:       true,
    ImportStateVerify: true,
}
```

For composite IDs, override `ImportStateIdFunc`:

```go
resource.TestStep{
    ResourceName:      "myprov_thing.test",
    ImportState:       true,
    ImportStateVerify: true,
    ImportStateIdFunc: func(s *terraform.State) (string, error) {
        rs := s.RootModule().Resources["myprov_thing.test"]
        return fmt.Sprintf("%s/%s", rs.Primary.Attributes["region"], rs.Primary.ID), nil
    },
}
```

If the import discards or transforms attributes (e.g., a write-only field that can't be read back), use `ImportStateVerifyIgnore` to exclude them from the comparison.
