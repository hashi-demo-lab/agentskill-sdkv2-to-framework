package openstack

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/gophercloud/gophercloud/v2/openstack/objectstorage/v1/containers"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// ---------------------------------------------------------------------------
// Unit tests — state upgrader (no network / TF_ACC required)
// ---------------------------------------------------------------------------

// TestObjectStorageContainerStateUpgradeV0_withVersioning verifies that V0
// state containing a populated "versioning" set is correctly upgraded to V1
// state: the set moves to "versioning_legacy" and "versioning" becomes false.
func TestObjectStorageContainerStateUpgradeV0_withVersioning(t *testing.T) {
	ctx := context.Background()

	priorSchema := priorSchemaV0ObjectStorageContainer()
	elemType := types.ObjectType{AttrTypes: versioningLegacyElemAttrTypes()}

	// Build the V0 "versioning" set (type=versions, location=othercontainer).
	versioningElem, diags := types.ObjectValue(versioningLegacyElemAttrTypes(), map[string]attr.Value{
		"type":     types.StringValue("versions"),
		"location": types.StringValue("othercontainer"),
	})
	if diags.HasError() {
		t.Fatalf("building versioning elem: %s", diags)
	}
	versioningSet, diags := types.SetValue(elemType, []attr.Value{versioningElem})
	if diags.HasError() {
		t.Fatalf("building versioning set: %s", diags)
	}

	metaMap, diags := types.MapValue(types.StringType, map[string]attr.Value{
		"test": types.StringValue("true"),
	})
	if diags.HasError() {
		t.Fatalf("building metadata map: %s", diags)
	}

	priorModel := objectStorageContainerModelV0{
		ID:               types.StringValue("testcontainer"),
		Region:           types.StringValue("RegionOne"),
		Name:             types.StringValue("testcontainer"),
		ContainerRead:    types.StringNull(),
		ContainerSyncTo:  types.StringNull(),
		ContainerSyncKey: types.StringNull(),
		ContainerWrite:   types.StringNull(),
		ContentType:      types.StringNull(),
		Versioning:       versioningSet,
		Metadata:         metaMap,
		ForceDestroy:     types.BoolValue(false),
		StoragePolicy:    types.StringNull(),
	}

	priorState := tfsdk.State{Schema: *priorSchema}
	if diags := priorState.Set(ctx, priorModel); diags.HasError() {
		t.Fatalf("setting prior state: %s", diags)
	}

	upgradeReq := fwresource.UpgradeStateRequest{State: &priorState}
	upgradeResp := &fwresource.UpgradeStateResponse{}

	upgradeObjectStorageContainerFromV0(ctx, upgradeReq, upgradeResp)

	if upgradeResp.Diagnostics.HasError() {
		t.Fatalf("upgrader returned errors: %s", upgradeResp.Diagnostics)
	}

	// Read result into current model.
	var got objectStorageContainerModel
	if diags := upgradeResp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("reading upgraded state: %s", diags)
	}

	// "versioning" must be bool false.
	if got.Versioning.ValueBool() {
		t.Errorf("expected versioning=false after upgrade, got true")
	}

	// "versioning_legacy" must contain the original entry.
	var vlEntries []versioningLegacyEntry
	if diags := got.VersioningLegacy.ElementsAs(ctx, &vlEntries, false); diags.HasError() {
		t.Fatalf("reading versioning_legacy: %s", diags)
	}
	if len(vlEntries) != 1 {
		t.Fatalf("expected 1 versioning_legacy entry, got %d", len(vlEntries))
	}
	if vlEntries[0].Type.ValueString() != "versions" {
		t.Errorf("expected versioning_legacy[0].type=versions, got %q", vlEntries[0].Type.ValueString())
	}
	if vlEntries[0].Location.ValueString() != "othercontainer" {
		t.Errorf("expected versioning_legacy[0].location=othercontainer, got %q", vlEntries[0].Location.ValueString())
	}

	// Scalar fields must be preserved.
	if got.ID.ValueString() != "testcontainer" {
		t.Errorf("expected id=testcontainer, got %q", got.ID.ValueString())
	}
	if got.Name.ValueString() != "testcontainer" {
		t.Errorf("expected name=testcontainer, got %q", got.Name.ValueString())
	}
	if got.Region.ValueString() != "RegionOne" {
		t.Errorf("expected region=RegionOne, got %q", got.Region.ValueString())
	}

	// "storage_class" was not present in V0; must default to empty string.
	if got.StorageClass.ValueString() != "" {
		t.Errorf("expected storage_class='', got %q", got.StorageClass.ValueString())
	}
}

// TestObjectStorageContainerStateUpgradeV0_emptyVersioning verifies that V0
// state with an empty "versioning" set results in an empty "versioning_legacy".
func TestObjectStorageContainerStateUpgradeV0_emptyVersioning(t *testing.T) {
	ctx := context.Background()

	priorSchema := priorSchemaV0ObjectStorageContainer()
	elemType := types.ObjectType{AttrTypes: versioningLegacyElemAttrTypes()}

	emptySet, diags := types.SetValue(elemType, []attr.Value{})
	if diags.HasError() {
		t.Fatalf("building empty versioning set: %s", diags)
	}
	emptyMeta, diags := types.MapValue(types.StringType, map[string]attr.Value{})
	if diags.HasError() {
		t.Fatalf("building empty metadata: %s", diags)
	}

	priorModel := objectStorageContainerModelV0{
		ID:            types.StringValue("ctr"),
		Region:        types.StringValue("RegionOne"),
		Name:          types.StringValue("ctr"),
		Versioning:    emptySet,
		Metadata:      emptyMeta,
		ForceDestroy:  types.BoolValue(false),
		StoragePolicy: types.StringNull(),
	}

	priorState := tfsdk.State{Schema: *priorSchema}
	if diags := priorState.Set(ctx, priorModel); diags.HasError() {
		t.Fatalf("setting prior state: %s", diags)
	}

	upgradeReq := fwresource.UpgradeStateRequest{State: &priorState}
	upgradeResp := &fwresource.UpgradeStateResponse{}

	upgradeObjectStorageContainerFromV0(ctx, upgradeReq, upgradeResp)

	if upgradeResp.Diagnostics.HasError() {
		t.Fatalf("upgrader returned errors: %s", upgradeResp.Diagnostics)
	}

	var got objectStorageContainerModel
	if diags := upgradeResp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("reading upgraded state: %s", diags)
	}

	if got.Versioning.ValueBool() {
		t.Error("expected versioning=false, got true")
	}
	if n := len(got.VersioningLegacy.Elements()); n != 0 {
		t.Errorf("expected empty versioning_legacy, got %d elements", n)
	}
}

// TestObjectStorageContainerStateUpgradeV0_historyType verifies the upgrader
// preserves a "history" type entry in versioning_legacy.
func TestObjectStorageContainerStateUpgradeV0_historyType(t *testing.T) {
	ctx := context.Background()

	priorSchema := priorSchemaV0ObjectStorageContainer()
	elemType := types.ObjectType{AttrTypes: versioningLegacyElemAttrTypes()}

	histElem, diags := types.ObjectValue(versioningLegacyElemAttrTypes(), map[string]attr.Value{
		"type":     types.StringValue("history"),
		"location": types.StringValue("historycontainer"),
	})
	if diags.HasError() {
		t.Fatalf("building history elem: %s", diags)
	}
	versioningSet, diags := types.SetValue(elemType, []attr.Value{histElem})
	if diags.HasError() {
		t.Fatalf("building versioning set: %s", diags)
	}
	emptyMeta, _ := types.MapValue(types.StringType, map[string]attr.Value{})

	priorModel := objectStorageContainerModelV0{
		ID:            types.StringValue("ctr"),
		Region:        types.StringValue("RegionOne"),
		Name:          types.StringValue("ctr"),
		Versioning:    versioningSet,
		Metadata:      emptyMeta,
		ForceDestroy:  types.BoolValue(false),
		StoragePolicy: types.StringNull(),
	}

	priorState := tfsdk.State{Schema: *priorSchema}
	if diags := priorState.Set(ctx, priorModel); diags.HasError() {
		t.Fatalf("setting prior state: %s", diags)
	}

	upgradeReq := fwresource.UpgradeStateRequest{State: &priorState}
	upgradeResp := &fwresource.UpgradeStateResponse{}

	upgradeObjectStorageContainerFromV0(ctx, upgradeReq, upgradeResp)

	if upgradeResp.Diagnostics.HasError() {
		t.Fatalf("upgrader returned errors: %s", upgradeResp.Diagnostics)
	}

	var got objectStorageContainerModel
	if diags := upgradeResp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("reading upgraded state: %s", diags)
	}

	var vlEntries []versioningLegacyEntry
	if diags := got.VersioningLegacy.ElementsAs(ctx, &vlEntries, false); diags.HasError() {
		t.Fatalf("reading versioning_legacy: %s", diags)
	}
	if len(vlEntries) != 1 {
		t.Fatalf("expected 1 versioning_legacy entry, got %d", len(vlEntries))
	}
	if vlEntries[0].Type.ValueString() != "history" {
		t.Errorf("expected type=history, got %q", vlEntries[0].Type.ValueString())
	}
	if vlEntries[0].Location.ValueString() != "historycontainer" {
		t.Errorf("expected location=historycontainer, got %q", vlEntries[0].Location.ValueString())
	}
}

// ---------------------------------------------------------------------------
// Acceptance tests — require TF_ACC=1 + OpenStack credentials
// ---------------------------------------------------------------------------

func TestAccObjectStorageV1Container_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheckNonAdminOnly(t)
			testAccPreCheckSwift(t)
		},
		// Use the framework provider factories after migration.
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

// TestAccObjectStorageV1Container_importBasic verifies that a container created
// with the framework provider can be imported and produces no diff.
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

// TestAccObjectStorageV1Container_stateUpgrade exercises the V0→V1 state
// upgrader end-to-end: step 1 writes V0 state using the last published SDKv2
// release; step 2 uses the migrated framework provider and asserts no plan diff.
//
// This test requires TF_ACC=1 and network access to the Terraform registry.
func TestAccObjectStorageV1Container_stateUpgrade(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheckNonAdminOnly(t)
			testAccPreCheckSwift(t)
		},
		// ProtoV6ProviderFactories applies to steps without ExternalProviders.
		// Replace with the provider's actual ProtoV6ProviderFactories var once
		// the provider-level framework registration is complete.
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckObjectStorageV1ContainerDestroy(t.Context()),
		Steps: []resource.TestStep{
			{
				// Step 1: write V0 state with the last published SDKv2 release.
				ExternalProviders: map[string]resource.ExternalProvider{
					"openstack": {
						VersionConstraint: "= 1.54.1",
						Source:            "registry.terraform.io/terraform-provider-openstack/openstack",
					},
				},
				Config: testAccObjectStorageV1ContainerBasic,
			},
			{
				// Step 2: framework provider; the UpgradeState upgrader runs and
				// must produce a state that matches the current schema with no diff.
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
