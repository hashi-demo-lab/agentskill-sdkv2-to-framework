# Data source migration

## Quick summary
- Data sources go from `*schema.Resource` (with the same shape as resources but only `ReadContext`) to a Go type implementing `datasource.DataSource`.
- Required methods: `Metadata`, `Schema`, `Read`. Optional: `Configure`, `ConfigValidators`, `ValidateConfig`.
- The schema package is `github.com/hashicorp/terraform-plugin-framework/datasource/schema` — do **not** import the resource package by mistake.
- Data sources have no `Create`/`Update`/`Delete`, no plan modifiers, and no state upgraders — strictly read-only.
- Like resources, all I/O is typed through a `tfsdk:"..."` model struct.

## Old shape (SDKv2)

In SDKv2, data sources are just `schema.Resource` with `ReadContext` only:

```go
func dataSourceThing() *schema.Resource {
    return &schema.Resource{
        ReadContext: dataSourceThingRead,
        Schema: map[string]*schema.Schema{
            "name":  {Type: schema.TypeString, Required: true},
            "value": {Type: schema.TypeString, Computed: true},
        },
    }
}
```

## New shape (framework)

```go
import (
    "context"
    "github.com/hashicorp/terraform-plugin-framework/datasource"
    "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
    "github.com/hashicorp/terraform-plugin-framework/types"
)

var (
    _ datasource.DataSource              = &thingDataSource{}
    _ datasource.DataSourceWithConfigure = &thingDataSource{}
)

func NewThingDataSource() datasource.DataSource { return &thingDataSource{} }

type thingDataSource struct {
    client *Client
}

type thingDataSourceModel struct {
    Name  types.String `tfsdk:"name"`
    Value types.String `tfsdk:"value"`
}

func (d *thingDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
    resp.TypeName = req.ProviderTypeName + "_thing"
}

func (d *thingDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
    resp.Schema = schema.Schema{
        Attributes: map[string]schema.Attribute{
            "name":  schema.StringAttribute{Required: true},
            "value": schema.StringAttribute{Computed: true},
        },
    }
}

func (d *thingDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
    if req.ProviderData == nil { return }
    client, ok := req.ProviderData.(*Client)
    if !ok {
        resp.Diagnostics.AddError("unexpected provider data", fmt.Sprintf("%T", req.ProviderData))
        return
    }
    d.client = client
}

func (d *thingDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
    var cfg thingDataSourceModel
    resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
    if resp.Diagnostics.HasError() { return }

    out, err := d.client.Lookup(cfg.Name.ValueString())
    if err != nil {
        resp.Diagnostics.AddError("lookup failed", err.Error())
        return
    }
    cfg.Value = types.StringValue(out.Value)
    resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}
```

## Read signature

| SDKv2 | Framework |
|---|---|
| `ReadContext func(ctx, d *schema.ResourceData, m interface{}) diag.Diagnostics` | `Read(ctx, req datasource.ReadRequest, resp *datasource.ReadResponse)` |

The data source's `Read` reads from `req.Config` (not `req.State` — there is no prior state for a data source) and writes to `resp.State`.

## Things data sources don't have

- No `Create`/`Update`/`Delete`.
- No plan modifiers in the schema (`PlanModifiers` is not a field on data-source attributes).
- No `Importer`.
- No state upgraders.
- No `Timeouts` field on the data source itself, though the framework's `timeouts` package provides a `Read` timeout if needed.
- No `Sensitive` *plan modifier* (the attribute-level `Sensitive: true` works on data sources too — see `sensitive-and-writeonly.md`).

## Common bug: importing resource schema by mistake

Because `resource/schema.StringAttribute` and `datasource/schema.StringAttribute` are different types, importing the wrong one causes errors like `cannot use schema.StringAttribute{} (type resource/schema.StringAttribute) as type datasource/schema.StringAttribute`.

Use named imports:
```go
import (
    rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
    dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)
```

## Data sources are usually the easy migrations

Migrate data sources first. They're smaller, have no state mutation, and exercise the schema and `Configure` plumbing without the full CRUD surface. A failure here usually points to a schema or provider-configure issue rather than data-source-specific logic.

## When the right answer is *not* a data source

If the SDKv2 data source is fetching short-lived credentials, OAuth tokens, vault-issued secrets, or anything else that should not be persisted to Terraform state, the framework-canonical shape in 2026 is an **ephemeral resource** (`ephemeral.EphemeralResource`), not a data source. Ephemeral resources implement `Open` / `Renew` / `Close` and never write to state — exactly the contract a credential-fetcher needs.

Ephemeral resources are out of scope for *this* skill (they're a framework-only feature with no SDKv2 equivalent to migrate from), but if your migration surfaces a data source whose value shouldn't be in state, signal that to the user: "this one shouldn't be a data source after the migration — it should be an ephemeral resource." Refer them to [HashiCorp's ephemeral-resource docs](https://developer.hashicorp.com/terraform/plugin/framework/ephemeral-resources) and treat the porting as a separate task.
