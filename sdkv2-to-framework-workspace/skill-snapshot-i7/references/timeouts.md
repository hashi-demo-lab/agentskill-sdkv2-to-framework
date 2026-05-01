# Timeouts

## Quick summary
- SDKv2's `Timeouts: &schema.ResourceTimeout{Create: ...}` field is gone. The framework moves timeouts to a separate package: `github.com/hashicorp/terraform-plugin-framework-timeouts`.
- Timeouts are exposed to practitioners as a nested attribute on the resource (`timeouts { create = "30m" }` block in HCL).
- Inside CRUD methods, you read the configured timeout from state/plan via the timeouts helper, then use it in your context (`context.WithTimeout`).
- Defaults are set on the schema attribute itself; per-operation timeouts (Create/Read/Update/Delete) are independent.
- This is opt-in — if your provider didn't have `Timeouts` before, you don't need to add it now.

## Old shape (SDKv2)

```go
&schema.Resource{
    Timeouts: &schema.ResourceTimeout{
        Create: schema.DefaultTimeout(30 * time.Minute),
        Update: schema.DefaultTimeout(30 * time.Minute),
        Delete: schema.DefaultTimeout(15 * time.Minute),
    },
    CreateContext: func(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
        // ctx already has the timeout deadline
        ...
    },
}
```

## New shape (framework)

Add the timeouts attribute to the schema:

```go
import (
    "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
)

func (r *thingResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
    resp.Schema = schema.Schema{
        Attributes: map[string]schema.Attribute{
            // your normal attributes...
            "timeouts": timeouts.Attributes(ctx, timeouts.Opts{
                Create: true,
                Update: true,
                Delete: true,
            }),
        },
    }
}
```

`Opts` controls which CRUD methods accept timeouts. The attribute renders in HCL as a block:

```hcl
resource "myprov_thing" "x" {
  name = "..."

  timeouts {
    create = "30m"
    update = "30m"
    delete = "15m"
  }
}
```

### `Block` vs `Attributes`

`timeouts.Attributes(ctx, opts)` renders as nested-attribute syntax (`timeouts = { create = "30m" }`). Most existing SDKv2 providers used block syntax for timeouts, so practitioners' configs are written as:

```hcl
timeouts {
  create = "30m"
}
```

To preserve that syntax across the migration, use `timeouts.Block(...)` instead — same `Opts` argument, but it goes into the schema's `Blocks:` map rather than `Attributes:`:

```go
resp.Schema = schema.Schema{
    Attributes: map[string]schema.Attribute{ /* ... */ },
    Blocks: map[string]schema.Block{
        "timeouts": timeouts.Block(ctx, timeouts.Opts{
            Create: true, Update: true, Delete: true,
        }),
    },
}
```

Pick `Attributes` for greenfield work where there's no backward-compat constraint; `Block` for migrations where existing configs use block syntax.

## Add timeouts field to your model

```go
import "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"

type thingModel struct {
    ID       types.String   `tfsdk:"id"`
    Name     types.String   `tfsdk:"name"`
    Timeouts timeouts.Value `tfsdk:"timeouts"`
}
```

## Read the timeout in CRUD methods

```go
func (r *thingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
    var plan thingModel
    resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
    if resp.Diagnostics.HasError() { return }

    createTimeout, diags := plan.Timeouts.Create(ctx, 30*time.Minute) // default
    resp.Diagnostics.Append(diags...)
    if resp.Diagnostics.HasError() { return }

    ctx, cancel := context.WithTimeout(ctx, createTimeout)
    defer cancel()

    // ... do the API calls using ctx ...
}
```

The methods on `timeouts.Value` are `Create(ctx, default)`, `Read(ctx, default)`, `Update(ctx, default)`, `Delete(ctx, default)`. Each returns a `time.Duration` (the configured value or the default if not set) and any diagnostics.

## Default timeouts

Set the default in your code, not in the schema. The schema attribute is `Optional`; if the practitioner doesn't set a timeout, you fall back to the default you pass to `plan.Timeouts.Create(ctx, 30*time.Minute)`.

## Data source timeouts

Data sources use a slightly different package and only support `Read`:

```go
import "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"

// in Schema
"timeouts": timeouts.Attributes(ctx, timeouts.Opts{Read: true})
```

## When to skip the migration

If your SDKv2 provider didn't define `Timeouts:`, don't add timeouts during migration. It's a feature, not a requirement. Adding it is also a (subtly) user-visible change — users gain the ability to write a `timeouts` block — and may merit changelog entries. Keep migrations as pure refactor where possible.
