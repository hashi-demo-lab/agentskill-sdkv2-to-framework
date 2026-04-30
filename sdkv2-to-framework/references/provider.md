# Provider definition migration

## Quick summary
- SDKv2 providers are a configured `*schema.Provider` value; framework providers are a Go type implementing the `provider.Provider` interface.
- Required methods on the provider type: `Metadata`, `Schema`, `Configure`, `Resources`, `DataSources`. Optional: `Functions`, `EphemeralResources`, etc.
- `ConfigureContextFunc` becomes the `Configure` method, with typed `req.Config.Get(...)` instead of `d.Get`.
- `ResourcesMap` / `DataSourcesMap` become `Resources` / `DataSources` returning slices of constructor functions.
- `main.go` switches from `plugin.Serve` to `providerserver.Serve` — see `protocol-versions.md`.

## Old shape (SDKv2)

```go
func New() *schema.Provider {
    return &schema.Provider{
        Schema: map[string]*schema.Schema{
            "region": {Type: schema.TypeString, Optional: true},
        },
        ResourcesMap: map[string]*schema.Resource{
            "myprov_thing": resourceThing(),
        },
        DataSourcesMap: map[string]*schema.Resource{
            "myprov_thing": dataSourceThing(),
        },
        ConfigureContextFunc: configure,
    }
}
```

## New shape (framework)

```go
type myProvider struct {
    version string
}

type myProviderModel struct {
    Region types.String `tfsdk:"region"`
}

func New(version string) func() provider.Provider {
    return func() provider.Provider { return &myProvider{version: version} }
}

func (p *myProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
    resp.TypeName = "myprov"
    resp.Version = p.version
}

func (p *myProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
    resp.Schema = schema.Schema{
        Attributes: map[string]schema.Attribute{
            "region": schema.StringAttribute{Optional: true},
        },
    }
}

func (p *myProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
    var data myProviderModel
    resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
    if resp.Diagnostics.HasError() {
        return
    }

    client, err := newClient(data.Region.ValueString())
    if err != nil {
        resp.Diagnostics.AddError("client init failed", err.Error())
        return
    }
    resp.DataSourceData = client
    resp.ResourceData = client
}

func (p *myProvider) Resources(ctx context.Context) []func() resource.Resource {
    return []func() resource.Resource{
        NewThingResource,
    }
}

func (p *myProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
    return []func() datasource.DataSource{
        NewThingDataSource,
    }
}
```

## Method-by-method translation

| SDKv2 | Framework |
|---|---|
| `&schema.Provider{Schema: ...}` | `(p) Schema(ctx, req, resp)` |
| `&schema.Provider{ResourcesMap: ...}` | `(p) Resources(ctx) []func() resource.Resource` |
| `&schema.Provider{DataSourcesMap: ...}` | `(p) DataSources(ctx) []func() datasource.DataSource` |
| `ConfigureContextFunc func(ctx, *schema.ResourceData) (interface{}, diag.Diagnostics)` | `(p) Configure(ctx, req, resp *provider.ConfigureResponse)` |
| `ProviderMetaSchema` | `(p) MetaSchema(ctx, req, resp)` (rarely used) |
| `TerraformVersion` (read in Configure) | `req.TerraformVersion` |

## Configure: passing data to resources/data sources

In SDKv2 the `Configure` function returned an `interface{}` "meta" that became `m` in every resource's CRUD function. In the framework, you set `resp.DataSourceData` and `resp.ResourceData` separately (often to the same client). Each resource's `Configure` method then receives `req.ProviderData` and type-asserts it.

```go
// In the resource
func (r *thingResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
    if req.ProviderData == nil { return } // called twice; once with nil
    client, ok := req.ProviderData.(*Client)
    if !ok {
        resp.Diagnostics.AddError("unexpected provider data type", fmt.Sprintf("got %T", req.ProviderData))
        return
    }
    r.client = client
}
```

The `req.ProviderData == nil` check is important — Terraform calls `Configure` once before the provider's own `Configure`, then again afterward. The first call has `nil`.

## Functional vs non-functional fields

The framework provider type holds *no functional state* between calls in normal use. Stash the configured client in the resource type after `Configure`. Don't store request-scoped state on the provider type — providers can be reused across many requests.

`p.version` is fine because it's set at construction and never changes.

## main.go

See `protocol-versions.md`. The summary: `plugin.Serve(&plugin.ServeOpts{...})` → `providerserver.Serve(ctx, providerNew, providerserver.ServeOpts{Address: "registry.terraform.io/.../myprov", ProtocolVersion: 6})`.
