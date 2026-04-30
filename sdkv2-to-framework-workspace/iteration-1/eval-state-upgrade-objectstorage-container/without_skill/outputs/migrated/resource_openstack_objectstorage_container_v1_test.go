package openstack

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/gophercloud/gophercloud/v2/openstack/objectstorage/v1/containers"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// ---------------------------------------------------------------------------
// Acceptance tests
// ---------------------------------------------------------------------------

func TestAccObjectStorageV1Container_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheckNonAdminOnly(t)
			testAccPreCheckSwift(t)
		},
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckObjectStorageV1ContainerDestroy(t.Context()),
		Steps: []resource.TestStep{
			{
				Config: testAccObjectStorageV1ContainerBasic,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(
						"openstack_objectstorage_container_v1.container_1", "name", "container_1"),
					resource.TestCheckResourceAttr(
						"openstack_objectstorage_container_v1.container_1", "metadata.test", "true"),
					resource.TestCheckResourceAttr(
						"openstack_objectstorage_container_v1.container_1", "metadata.upperTest", "true"),
					resource.TestCheckResourceAttr(
						"openstack_objectstorage_container_v1.container_1", "content_type", "application/json"),
				),
			},
			{
				Config: testAccObjectStorageV1ContainerUpdate,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(
						"openstack_objectstorage_container_v1.container_1", "content_type", "text/plain"),
				),
			},
		},
	})
}

func TestAccObjectStorageV1Container_versioning(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheckNonAdminOnly(t)
			testAccPreCheckSwift(t)
		},
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckObjectStorageV1ContainerDestroy(t.Context()),
		Steps: []resource.TestStep{
			{
				Config: testAccObjectStorageV1ContainerVersioning,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(
						"openstack_objectstorage_container_v1.container_1", "name", "container_1"),
					resource.TestCheckResourceAttr(
						"openstack_objectstorage_container_v1.container_1", "versioning", "true"),
					resource.TestCheckResourceAttr(
						"openstack_objectstorage_container_v1.container_1", "metadata.test", "true"),
					resource.TestCheckResourceAttr(
						"openstack_objectstorage_container_v1.container_1", "metadata.upperTest", "true"),
					resource.TestCheckResourceAttr(
						"openstack_objectstorage_container_v1.container_1", "content_type", "application/json"),
				),
			},
		},
	})
}

func TestAccObjectStorageV1Container_storagePolicy(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheckNonAdminOnly(t)
			testAccPreCheckSwift(t)
		},
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckObjectStorageV1ContainerDestroy(t.Context()),
		Steps: []resource.TestStep{
			{
				Config: testAccObjectStorageV1ContainerStoragePolicy,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(
						"openstack_objectstorage_container_v1.container_1", "name", "container_1"),
					resource.TestCheckResourceAttr(
						"openstack_objectstorage_container_v1.container_1", "metadata.test", "true"),
					resource.TestCheckResourceAttr(
						"openstack_objectstorage_container_v1.container_1", "metadata.upperTest", "true"),
					resource.TestCheckResourceAttr(
						"openstack_objectstorage_container_v1.container_1", "content_type", "application/json"),
					resource.TestCheckResourceAttr(
						"openstack_objectstorage_container_v1.container_1", "storage_policy", "Policy-0"),
				),
			},
		},
	})
}

func TestAccObjectStorageV1Container_importBasic(t *testing.T) {
	resourceName := "openstack_objectstorage_container_v1.container_1"

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheckNonAdminOnly(t)
			testAccPreCheckSwift(t)
		},
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckObjectStorageV1ContainerDestroy(t.Context()),
		Steps: []resource.TestStep{
			{
				Config: testAccObjectStorageV1ContainerComplete,
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"force_destroy",
					"content_type",
					"metadata",
				},
			},
		},
	})
}

func testAccCheckObjectStorageV1ContainerDestroy(ctx context.Context) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		config := testAccProvider.Meta().(*Config)

		objectStorageClient, err := config.ObjectStorageV1Client(ctx, osRegionName)
		if err != nil {
			return fmt.Errorf("Error creating OpenStack object storage client: %w", err)
		}

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "openstack_objectstorage_container_v1" {
				continue
			}

			_, err := containers.Get(ctx, objectStorageClient, rs.Primary.ID, nil).Extract()
			if err == nil {
				return errors.New("Container still exists")
			}
		}

		return nil
	}
}

// ---------------------------------------------------------------------------
// Unit test: UpgradeState V0 → V1
// ---------------------------------------------------------------------------

// TestObjectStorageContainerStateUpgradeV0 tests the framework UpgradeState
// handler that replaces the SDK v2 resourceObjectStorageContainerStateUpgradeV0
// function.  It verifies:
//   - the old "versioning" block list is moved to "versioning_legacy"
//   - the new "versioning" bool is set to false
//   - all other fields are preserved unchanged
func TestObjectStorageContainerStateUpgradeV0(t *testing.T) {
	ctx := context.Background()

	objType := types.ObjectType{AttrTypes: versioningLegacyAttrTypes()}

	// Build a raw V0 state that matches PriorSchema:
	//   versioning = [{ type = "versions", location = "test" }]
	versioningObj, _ := types.ObjectValue(versioningLegacyAttrTypes(), map[string]attr.Value{
		"type":     types.StringValue("versions"),
		"location": types.StringValue("test"),
	})
	v0VersioningList := types.ListValueMust(objType, []attr.Value{versioningObj})

	// Construct the prior state using the PriorSchema defined in UpgradeState.
	res := &objectStorageContainerV1Resource{}
	upgraders := res.UpgradeState(ctx)
	upgrader, ok := upgraders[0]
	if !ok {
		t.Fatal("expected upgrader for version 0 to be registered")
	}

	// We need to build a tfsdk.State from the prior schema + raw values.
	priorSchema := upgrader.PriorSchema

	// Build the raw tftypes.Value for the prior state.
	priorRaw := tftypes.NewValue(
		priorSchema.Type().TerraformType(ctx),
		map[string]tftypes.Value{
			"id":                 tftypes.NewValue(tftypes.String, "test"),
			"region":             tftypes.NewValue(tftypes.String, "RegionOne"),
			"name":               tftypes.NewValue(tftypes.String, "test"),
			"container_read":     tftypes.NewValue(tftypes.String, nil),
			"container_sync_to":  tftypes.NewValue(tftypes.String, nil),
			"container_sync_key": tftypes.NewValue(tftypes.String, nil),
			"container_write":    tftypes.NewValue(tftypes.String, nil),
			"content_type":       tftypes.NewValue(tftypes.String, nil),
			"versioning": tftypes.NewValue(
				tftypes.List{ElementType: tftypes.Object{
					AttributeTypes: map[string]tftypes.Type{
						"type":     tftypes.String,
						"location": tftypes.String,
					},
				}},
				[]tftypes.Value{
					tftypes.NewValue(tftypes.Object{
						AttributeTypes: map[string]tftypes.Type{
							"type":     tftypes.String,
							"location": tftypes.String,
						},
					}, map[string]tftypes.Value{
						"type":     tftypes.NewValue(tftypes.String, "versions"),
						"location": tftypes.NewValue(tftypes.String, "test"),
					}),
				},
			),
			"metadata":       tftypes.NewValue(tftypes.Map{ElementType: tftypes.String}, map[string]tftypes.Value{}),
			"force_destroy":  tftypes.NewValue(tftypes.Bool, false),
			"storage_policy": tftypes.NewValue(tftypes.String, nil),
		},
	)

	priorState := tfsdk.State{
		Schema: *priorSchema,
		Raw:    priorRaw,
	}

	// Run the upgrader.
	upgradeReq := fwresource.UpgradeStateRequest{State: &priorState}
	upgradeResp := &fwresource.UpgradeStateResponse{
		State: tfsdk.State{Schema: schemaFromResource(ctx, res)},
	}
	upgrader.StateUpgrader(ctx, upgradeReq, upgradeResp)

	if upgradeResp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %s", upgradeResp.Diagnostics)
	}

	// Extract and verify the upgraded state.
	var upgraded containerV1Model
	if d := upgradeResp.State.Get(ctx, &upgraded); d.HasError() {
		t.Fatalf("failed to get upgraded state: %s", d)
	}

	// versioning (bool) must be false.
	if upgraded.Versioning.ValueBool() != false {
		t.Errorf("expected versioning=false, got %v", upgraded.Versioning.ValueBool())
	}

	// versioning_legacy must contain the moved block.
	if !upgraded.VersioningLegacy.Equal(v0VersioningList) {
		t.Errorf("expected versioning_legacy=%v, got %v", v0VersioningList, upgraded.VersioningLegacy)
	}

	// name preserved.
	if upgraded.Name.ValueString() != "test" {
		t.Errorf("expected name=test, got %q", upgraded.Name.ValueString())
	}
}

// schemaFromResource is a test helper that retrieves the current schema from
// the resource so we can build an empty upgraded tfsdk.State.
func schemaFromResource(ctx context.Context, r *objectStorageContainerV1Resource) schema.Schema {
	req := fwresource.SchemaRequest{}
	resp := &fwresource.SchemaResponse{}
	r.Schema(ctx, req, resp)
	return resp.Schema
}

// ---------------------------------------------------------------------------
// Terraform configuration fixtures
// ---------------------------------------------------------------------------

const testAccObjectStorageV1ContainerBasic = `
resource "openstack_objectstorage_container_v1" "container_1" {
  name = "container_1"
  metadata = {
    test      = "true"
    upperTest = "true"
  }
  content_type = "application/json"
}
`

const testAccObjectStorageV1ContainerComplete = `
resource "openstack_objectstorage_container_v1" "container_1" {
  name = "container_1"
  metadata = {
    test      = "true"
    upperTest = "true"
  }
  content_type = "application/json"
  versioning_legacy {
    type     = "versions"
    location = "othercontainer"
  }
  container_read  = ".r:*,.rlistings"
  container_write = "*"
}
`

const testAccObjectStorageV1ContainerVersioning = `
resource "openstack_objectstorage_container_v1" "container_1" {
  name = "container_1"
  metadata = {
    test      = "true"
    upperTest = "true"
  }
  content_type    = "application/json"
  versioning      = true
  container_read  = ".r:*,.rlistings"
  container_write = "*"
}
`

const testAccObjectStorageV1ContainerUpdate = `
resource "openstack_objectstorage_container_v1" "container_1" {
  name = "container_1"
  metadata = {
    test = "true"
  }
  content_type = "text/plain"
}
`

const testAccObjectStorageV1ContainerStoragePolicy = `
resource "openstack_objectstorage_container_v1" "container_1" {
  name = "container_1"
  metadata = {
    test      = "true"
    upperTest = "true"
  }
  content_type   = "application/json"
  storage_policy = "Policy-0"
}
`
