// Synthetic fixture for eval-chained-upgraders.
//
// SDKv2 resource at SchemaVersion: 2 with two CHAINED state upgraders
// (V0 → V1, V1 → V2). The migration must produce a framework
// ResourceWithUpgradeState whose UpgradeState() map has TWO entries —
// `0:` and `1:` — each producing the CURRENT (V2) state in one call.
// V0 must NOT call V1's upgrader inside its body.

package widget

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func ResourceWidget() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceWidgetCreate,
		ReadContext:   resourceWidgetRead,
		UpdateContext: resourceWidgetUpdate,
		DeleteContext: resourceWidgetDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		SchemaVersion: 2,
		StateUpgraders: []schema.StateUpgrader{
			{
				Version: 0,
				Type:    resourceWidgetV0().CoreConfigSchema().ImpliedType(),
				Upgrade: upgradeWidgetV0ToV1,
			},
			{
				Version: 1,
				Type:    resourceWidgetV1().CoreConfigSchema().ImpliedType(),
				Upgrade: upgradeWidgetV1ToV2,
			},
		},

		Schema: map[string]*schema.Schema{
			"id": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			// V2 split the legacy "host_port" string into typed fields.
			"host": {
				Type:     schema.TypeString,
				Required: true,
			},
			"port": {
				Type:     schema.TypeInt,
				Required: true,
			},
			// V2 added tags.
			"tags": {
				Type:     schema.TypeMap,
				Optional: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
		},
	}
}

// V0: name only, plus a legacy "address" field that mixed host:port.
func resourceWidgetV0() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"id":      {Type: schema.TypeString, Computed: true},
			"name":    {Type: schema.TypeString, Required: true, ForceNew: true},
			"address": {Type: schema.TypeString, Required: true},
		},
	}
}

// V1: split "address" → "host_port" (still a single string), kept "name".
func resourceWidgetV1() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"id":        {Type: schema.TypeString, Computed: true},
			"name":      {Type: schema.TypeString, Required: true, ForceNew: true},
			"host_port": {Type: schema.TypeString, Required: true},
		},
	}
}

// V0 → V1: just rename "address" to "host_port".
func upgradeWidgetV0ToV1(ctx context.Context, raw map[string]interface{}, m interface{}) (map[string]interface{}, error) {
	if addr, ok := raw["address"].(string); ok {
		raw["host_port"] = addr
		delete(raw, "address")
	}
	return raw, nil
}

// V1 → V2: split "host_port" into "host" and "port"; tags default to empty map.
func upgradeWidgetV1ToV2(ctx context.Context, raw map[string]interface{}, m interface{}) (map[string]interface{}, error) {
	if hp, ok := raw["host_port"].(string); ok {
		parts := strings.SplitN(hp, ":", 2)
		raw["host"] = parts[0]
		if len(parts) == 2 {
			raw["port"] = parts[1] // upgrader emits string; framework will coerce when re-typed
		} else {
			raw["port"] = "0"
		}
		delete(raw, "host_port")
	}
	if _, ok := raw["tags"]; !ok {
		raw["tags"] = map[string]interface{}{}
	}
	return raw, nil
}

// Stubs — content irrelevant to the migration (the upgrader logic is the focus).
func resourceWidgetCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics { return nil }
func resourceWidgetRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics   { return nil }
func resourceWidgetUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics { return nil }
func resourceWidgetDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics { return nil }
