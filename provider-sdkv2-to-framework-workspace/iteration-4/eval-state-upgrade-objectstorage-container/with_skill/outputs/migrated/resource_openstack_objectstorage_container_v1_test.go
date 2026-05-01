package openstack

// Migrated test file: SDKv2 → terraform-plugin-framework
//
// Changes from the original:
//   - ProviderFactories → ProtoV6ProviderFactories (protoV6ProviderFactories).
//   - Acceptance test bodies are otherwise structurally unchanged.
//   - State-upgrade unit test replaces migrate_resource_openstack_objectstorage_container_v1_test.go:
//     the old SDKv2 test called resourceObjectStorageContainerStateUpgradeV0 on
//     a raw map; this test constructs a typed tfsdk.State matching containerSchemaV0,
//     calls upgradeContainerStateV0toV1, and asserts the resulting V1 state — giving
//     compile-time type safety and proving PriorSchema / model tags are correct.

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/gophercloud/gophercloud/v2/openstack/objectstorage/v1/containers"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
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
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckObjectStorageV1ContainerDestroy(t.Context()),
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
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckObjectStorageV1ContainerDestroy(t.Context()),
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
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckObjectStorageV1ContainerDestroy(t.Context()),
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

// TestAccObjectStorageV1Container_importBasic exercises import round-trip.
func TestAccObjectStorageV1Container_importBasic(t *testing.T) {
	resourceName := "openstack_objectstorage_container_v1.container_1"

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheckNonAdminOnly(t)
			testAccPreCheckSwift(t)
		},
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckObjectStorageV1ContainerDestroy(t.Context()),
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

// ---------------------------------------------------------------------------
// State-upgrade unit test (replaces migrate_resource_openstack_objectstorage_container_v1_test.go)
// ---------------------------------------------------------------------------

// TestObjectStorageV1ContainerStateUpgradeV0 is a non-acceptance unit test
// that directly exercises upgradeContainerStateV0toV1 using the framework's
// typed state helpers.
//
// It constructs a fake V0 tfsdk.State using containerSchemaV0 and
// tftypes.Value, runs the upgrader, then asserts the resulting V1 model
// fields are correct. A mismatch between PriorSchema and the V0 model
// struct tags will fail at the Get() call rather than silently losing data.
func TestObjectStorageV1ContainerStateUpgradeV0(t *testing.T) {
	ctx := context.Background()

	// Build the V0 prior state matching containerSchemaV0.
	// "versioning" in V0 was a TypeSet block with type + location sub-fields.
	versioningElemType := tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"type":     tftypes.String,
			"location": tftypes.String,
		},
	}
	versioningBlockType := tftypes.Set{ElementType: versioningElemType}

	v0StateVal := tftypes.NewValue(tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"id":                 tftypes.String,
			"region":             tftypes.String,
			"name":               tftypes.String,
			"container_read":     tftypes.String,
			"container_sync_to":  tftypes.String,
			"container_sync_key": tftypes.String,
			"container_write":    tftypes.String,
			"content_type":       tftypes.String,
			"versioning":         versioningBlockType,
			"metadata":           tftypes.Map{ElementType: tftypes.String},
			"force_destroy":      tftypes.Bool,
			"storage_policy":     tftypes.String,
		},
	}, map[string]tftypes.Value{
		"id":                 tftypes.NewValue(tftypes.String, "test"),
		"region":             tftypes.NewValue(tftypes.String, "RegionOne"),
		"name":               tftypes.NewValue(tftypes.String, "test"),
		"container_read":     tftypes.NewValue(tftypes.String, nil),
		"container_sync_to":  tftypes.NewValue(tftypes.String, nil),
		"container_sync_key": tftypes.NewValue(tftypes.String, nil),
		"container_write":    tftypes.NewValue(tftypes.String, nil),
		"content_type":       tftypes.NewValue(tftypes.String, nil),
		"versioning": tftypes.NewValue(versioningBlockType, []tftypes.Value{
			tftypes.NewValue(versioningElemType, map[string]tftypes.Value{
				"type":     tftypes.NewValue(tftypes.String, "versions"),
				"location": tftypes.NewValue(tftypes.String, "test"),
			}),
		}),
		"metadata":       tftypes.NewValue(tftypes.Map{ElementType: tftypes.String}, map[string]tftypes.Value{}),
		"force_destroy":  tftypes.NewValue(tftypes.Bool, false),
		"storage_policy": tftypes.NewValue(tftypes.String, nil),
	})

	priorSchema := containerSchemaV0()
	priorState := tfsdk.State{
		Schema: *priorSchema,
		Raw:    v0StateVal,
	}

	// Build the upgrade response with the current V1 schema.
	r := &objectStorageContainerV1Resource{}
	var schemaResp fwresource.SchemaResponse
	r.Schema(ctx, fwresource.SchemaRequest{}, &schemaResp)

	upgradeReq := fwresource.UpgradeStateRequest{
		State: &priorState,
	}
	upgradeResp := &fwresource.UpgradeStateResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
		},
	}

	upgradeContainerStateV0toV1(ctx, upgradeReq, upgradeResp)
	if upgradeResp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics from upgrader: %s", upgradeResp.Diagnostics)
	}

	// Read the resulting state into a V1 model and assert field values.
	var result objectStorageContainerV1Model
	if diags := upgradeResp.State.Get(ctx, &result); diags.HasError() {
		t.Fatalf("failed to read upgraded state: %s", diags)
	}

	// versioning (bool) must be false after upgrade.
	if result.Versioning.ValueBool() {
		t.Errorf("expected versioning=false after upgrade, got true")
	}

	// versioning_legacy must carry the old versioning block data.
	if len(result.VersioningLegacy) != 1 {
		t.Fatalf(
			"expected 1 versioning_legacy entry after upgrade, got %d",
			len(result.VersioningLegacy),
		)
	}
	if got := result.VersioningLegacy[0].Type.ValueString(); got != "versions" {
		t.Errorf("expected versioning_legacy[0].type=%q, got %q", "versions", got)
	}
	if got := result.VersioningLegacy[0].Location.ValueString(); got != "test" {
		t.Errorf("expected versioning_legacy[0].location=%q, got %q", "test", got)
	}

	// name must be preserved.
	if got := result.Name.ValueString(); got != "test" {
		t.Errorf("expected name=%q, got %q", "test", got)
	}

	// region must be preserved.
	if got := result.Region.ValueString(); got != "RegionOne" {
		t.Errorf("expected region=%q, got %q", "RegionOne", got)
	}

	// storage_class must be null (field did not exist in V0).
	if !result.StorageClass.IsNull() {
		t.Errorf(
			"expected storage_class to be null after V0 upgrade, got %q",
			result.StorageClass.ValueString(),
		)
	}

	// force_destroy must be false.
	if result.ForceDestroy.ValueBool() {
		t.Errorf("expected force_destroy=false after upgrade, got true")
	}

	// Ensure types package import is used (guard against unused-import errors).
	_ = types.StringNull()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func testAccCheckObjectStorageV1ContainerDestroy(ctx context.Context) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		config := testAccProvider.Meta().(*Config)

		objectStorageClient, err := config.ObjectStorageV1Client(ctx, osRegionName)
		if err != nil {
			return fmt.Errorf("error creating OpenStack object storage client: %w", err)
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
// HCL configs (unchanged from original)
// ---------------------------------------------------------------------------

const testAccObjectStorageV1ContainerBasic = `
resource "openstack_objectstorage_container_v1" "container_1" {
  name = "container_1"
  metadata = {
    test = "true"
    upperTest = "true"
  }
  content_type = "application/json"
}
`

const testAccObjectStorageV1ContainerComplete = `
resource "openstack_objectstorage_container_v1" "container_1" {
  name = "container_1"
  metadata = {
    test = "true"
    upperTest = "true"
  }
  content_type = "application/json"
  versioning_legacy {
    type = "versions"
    location = "othercontainer"
  }
  container_read = ".r:*,.rlistings"
  container_write = "*"
}
`

const testAccObjectStorageV1ContainerVersioning = `
resource "openstack_objectstorage_container_v1" "container_1" {
  name = "container_1"
  metadata = {
    test = "true"
    upperTest = "true"
  }
  content_type = "application/json"
  versioning = true
  container_read = ".r:*,.rlistings"
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
    test = "true"
    upperTest = "true"
  }
  content_type = "application/json"
  storage_policy = "Policy-0"
}
`
