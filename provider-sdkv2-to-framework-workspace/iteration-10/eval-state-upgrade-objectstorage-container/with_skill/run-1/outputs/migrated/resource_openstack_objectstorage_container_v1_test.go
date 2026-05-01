package openstack

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
// Unit tests — state upgrade V0 → V1 (current)
// ---------------------------------------------------------------------------

// TestObjectStorageContainerV1UpgradeStateV0 verifies the single-step V0→V1
// state upgrader: the old "versioning" set moves to "versioning_legacy", and
// the new "versioning" bool becomes false.
func TestObjectStorageContainerV1UpgradeStateV0(t *testing.T) {
	t.Parallel()

	// Build a V0 raw state using tftypes so we can exercise the upgrader
	// end-to-end through the framework's deserialization path.
	res := &objectStorageContainerV1Resource{}
	upgraders := res.UpgradeState(context.Background())

	upgrader, ok := upgraders[0]
	if !ok {
		t.Fatal("expected upgrader for version 0")
	}

	// Craft a V0 tftypes.Value that matches the PriorSchema.
	priorStateType := tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"id":                 tftypes.String,
			"region":             tftypes.String,
			"name":               tftypes.String,
			"container_read":     tftypes.String,
			"container_sync_to":  tftypes.String,
			"container_sync_key": tftypes.String,
			"container_write":    tftypes.String,
			"content_type":       tftypes.String,
			"versioning": tftypes.Set{
				ElementType: tftypes.Object{
					AttributeTypes: map[string]tftypes.Type{
						"type":     tftypes.String,
						"location": tftypes.String,
					},
				},
			},
			"metadata":       tftypes.Map{ElementType: tftypes.String},
			"force_destroy":  tftypes.Bool,
			"storage_policy": tftypes.String,
		},
	}

	versioningItem := tftypes.NewValue(
		tftypes.Object{
			AttributeTypes: map[string]tftypes.Type{
				"type":     tftypes.String,
				"location": tftypes.String,
			},
		},
		map[string]tftypes.Value{
			"type":     tftypes.NewValue(tftypes.String, "versions"),
			"location": tftypes.NewValue(tftypes.String, "test-container"),
		},
	)

	priorStateVal := tftypes.NewValue(priorStateType, map[string]tftypes.Value{
		"id":                 tftypes.NewValue(tftypes.String, "mycontainer"),
		"region":             tftypes.NewValue(tftypes.String, "RegionOne"),
		"name":               tftypes.NewValue(tftypes.String, "mycontainer"),
		"container_read":     tftypes.NewValue(tftypes.String, ""),
		"container_sync_to":  tftypes.NewValue(tftypes.String, ""),
		"container_sync_key": tftypes.NewValue(tftypes.String, ""),
		"container_write":    tftypes.NewValue(tftypes.String, ""),
		"content_type":       tftypes.NewValue(tftypes.String, ""),
		"versioning": tftypes.NewValue(
			tftypes.Set{
				ElementType: tftypes.Object{
					AttributeTypes: map[string]tftypes.Type{
						"type":     tftypes.String,
						"location": tftypes.String,
					},
				},
			},
			[]tftypes.Value{versioningItem},
		),
		"metadata":       tftypes.NewValue(tftypes.Map{ElementType: tftypes.String}, map[string]tftypes.Value{}),
		"force_destroy":  tftypes.NewValue(tftypes.Bool, false),
		"storage_policy": tftypes.NewValue(tftypes.String, ""),
	})

	priorSchema := upgrader.PriorSchema
	upgradeReq := fwresource.UpgradeStateRequest{
		State: &tfsdk.State{
			Schema: *priorSchema,
			Raw:    priorStateVal,
		},
	}
	upgradeResp := &fwresource.UpgradeStateResponse{
		State: tfsdk.State{},
	}

	// Call the upgrader.
	upgrader.StateUpgrader(context.Background(), upgradeReq, upgradeResp)
	if upgradeResp.Diagnostics.HasError() {
		t.Fatalf("upgrader returned errors: %s", upgradeResp.Diagnostics)
	}

	// Read the upgraded state back into a containerModel.
	var upgraded containerModel
	diags := upgradeResp.State.Get(context.Background(), &upgraded)
	if diags.HasError() {
		t.Fatalf("failed to read upgraded state: %s", diags)
	}

	// "versioning" must be false.
	if !upgraded.Versioning.Equal(types.BoolValue(false)) {
		t.Errorf("expected versioning=false, got %v", upgraded.Versioning)
	}

	// "versioning_legacy" must contain one element with type="versions".
	if upgraded.VersioningLegacy.IsNull() || upgraded.VersioningLegacy.IsUnknown() {
		t.Fatal("expected versioning_legacy to be set, got null/unknown")
	}
	vItems := make([]versioningLegacyModel, 0)
	diags = upgraded.VersioningLegacy.ElementsAs(context.Background(), &vItems, false)
	if diags.HasError() {
		t.Fatalf("failed to extract versioning_legacy elements: %s", diags)
	}
	if len(vItems) != 1 {
		t.Fatalf("expected 1 versioning_legacy item, got %d", len(vItems))
	}
	if vItems[0].Type.ValueString() != "versions" {
		t.Errorf("expected versioning_legacy[0].type=versions, got %s", vItems[0].Type.ValueString())
	}
	if vItems[0].Location.ValueString() != "test-container" {
		t.Errorf("expected versioning_legacy[0].location=test-container, got %s", vItems[0].Location.ValueString())
	}

	// storage_class must default to "".
	if upgraded.StorageClass.ValueString() != "" {
		t.Errorf("expected storage_class empty, got %s", upgraded.StorageClass.ValueString())
	}
}

// TestObjectStorageContainerV1UpgradeStateV0_NoVersioning verifies upgrade
// when the prior state had an empty versioning set.
func TestObjectStorageContainerV1UpgradeStateV0_NoVersioning(t *testing.T) {
	t.Parallel()

	res := &objectStorageContainerV1Resource{}
	upgraders := res.UpgradeState(context.Background())
	upgrader := upgraders[0]

	priorStateType := tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"id":                 tftypes.String,
			"region":             tftypes.String,
			"name":               tftypes.String,
			"container_read":     tftypes.String,
			"container_sync_to":  tftypes.String,
			"container_sync_key": tftypes.String,
			"container_write":    tftypes.String,
			"content_type":       tftypes.String,
			"versioning": tftypes.Set{
				ElementType: tftypes.Object{
					AttributeTypes: map[string]tftypes.Type{
						"type":     tftypes.String,
						"location": tftypes.String,
					},
				},
			},
			"metadata":       tftypes.Map{ElementType: tftypes.String},
			"force_destroy":  tftypes.Bool,
			"storage_policy": tftypes.String,
		},
	}

	priorStateVal := tftypes.NewValue(priorStateType, map[string]tftypes.Value{
		"id":                 tftypes.NewValue(tftypes.String, "mycontainer"),
		"region":             tftypes.NewValue(tftypes.String, "RegionOne"),
		"name":               tftypes.NewValue(tftypes.String, "mycontainer"),
		"container_read":     tftypes.NewValue(tftypes.String, ""),
		"container_sync_to":  tftypes.NewValue(tftypes.String, ""),
		"container_sync_key": tftypes.NewValue(tftypes.String, ""),
		"container_write":    tftypes.NewValue(tftypes.String, ""),
		"content_type":       tftypes.NewValue(tftypes.String, ""),
		"versioning": tftypes.NewValue(
			tftypes.Set{
				ElementType: tftypes.Object{
					AttributeTypes: map[string]tftypes.Type{
						"type":     tftypes.String,
						"location": tftypes.String,
					},
				},
			},
			[]tftypes.Value{},
		),
		"metadata":       tftypes.NewValue(tftypes.Map{ElementType: tftypes.String}, map[string]tftypes.Value{}),
		"force_destroy":  tftypes.NewValue(tftypes.Bool, false),
		"storage_policy": tftypes.NewValue(tftypes.String, ""),
	})

	priorSchema := upgrader.PriorSchema
	upgradeReq2 := fwresource.UpgradeStateRequest{
		State: &tfsdk.State{
			Schema: *priorSchema,
			Raw:    priorStateVal,
		},
	}
	upgradeResp2 := &fwresource.UpgradeStateResponse{
		State: tfsdk.State{},
	}

	upgrader.StateUpgrader(context.Background(), upgradeReq2, upgradeResp2)
	if upgradeResp2.Diagnostics.HasError() {
		t.Fatalf("upgrader returned errors: %s", upgradeResp2.Diagnostics)
	}

	var upgraded containerModel
	diags := upgradeResp2.State.Get(context.Background(), &upgraded)
	if diags.HasError() {
		t.Fatalf("failed to read upgraded state: %s", diags)
	}

	if !upgraded.Versioning.Equal(types.BoolValue(false)) {
		t.Errorf("expected versioning=false, got %v", upgraded.Versioning)
	}

	if upgraded.VersioningLegacy.IsNull() {
		t.Fatal("versioning_legacy should be an empty set, not null")
	}

	if len(upgraded.VersioningLegacy.Elements()) != 0 {
		t.Errorf("expected empty versioning_legacy, got %d elements", len(upgraded.VersioningLegacy.Elements()))
	}
}

// ---------------------------------------------------------------------------
// Acceptance tests
// ---------------------------------------------------------------------------

func TestAccObjectStorageV1Container_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheckNonAdminOnly(t)
			testAccPreCheckSwift(t)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
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
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
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
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
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

// TestAccObjectStorageV1Container_importBasic verifies import state works.
func TestAccObjectStorageV1Container_importBasic(t *testing.T) {
	resourceName := "openstack_objectstorage_container_v1.container_1"

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheckNonAdminOnly(t)
			testAccPreCheckSwift(t)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
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

// TestAccObjectStorageV1Container_stateUpgradeV0 exercises the V0→V1 state
// upgrade path in an acceptance test by writing V0 state with the last SDKv2
// provider release, then applying the migrated framework provider and asserting
// no plan diff.
func TestAccObjectStorageV1Container_stateUpgradeV0(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// Step 1: write V0 state using the last published SDKv2 release.
				ExternalProviders: map[string]resource.ExternalProvider{
					"openstack": {
						VersionConstraint: "= 1.54.1",
						Source:            "registry.terraform.io/terraform-provider-openstack/openstack",
					},
				},
				Config: testAccObjectStorageV1ContainerBasic,
			},
			{
				// Step 2: framework provider; assert no plan diff after upgrade.
				Config:   testAccObjectStorageV1ContainerBasic,
				PlanOnly: true,
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
