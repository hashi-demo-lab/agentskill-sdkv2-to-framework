package openstack

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/gophercloud/gophercloud/v2/openstack/objectstorage/v1/containers"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// protoV6ProviderFactories is the framework provider factory for acceptance tests.
// Replace NewFrameworkProvider() with the actual constructor once the full
// provider has been migrated to the framework.
var protoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"openstack": providerserver.NewProtocol6WithError(NewFrameworkProvider()),
}

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

// TestAccObjectStorageV1Container_stateUpgradeFromV0 verifies that state
// written by the SDKv2 provider at schema version 0 is correctly upgraded to
// schema version 1 by the framework provider's UpgradeState map (single-step
// semantics).
//
// The first step applies a V0 config using the last published SDKv2 release
// via ExternalProviders; the second step uses the framework provider
// (ProtoV6ProviderFactories at TestCase level) and asserts an empty plan
// (no spurious diff after the upgrade).
func TestAccObjectStorageV1Container_stateUpgradeFromV0(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheckNonAdminOnly(t)
			testAccPreCheckSwift(t)
		},
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckObjectStorageV1ContainerDestroy(t.Context()),
		Steps: []resource.TestStep{
			{
				// Step 1: create the resource using the last SDKv2 release (V0 schema).
				// ExternalProviders overrides the TestCase-level factories for this
				// step only.
				ExternalProviders: map[string]resource.ExternalProvider{
					"openstack": {
						// Pin to the last SDKv2 release that used schema version 0.
						VersionConstraint: "~> 1.x",
						Source:            "registry.terraform.io/terraform-provider-openstack/openstack",
					},
				},
				Config: testAccObjectStorageV1ContainerBasic,
			},
			{
				// Step 2: the framework provider (TestCase-level factories) runs
				// UpgradeState 0→1 and then plans.  Expect no diff.
				Config:   testAccObjectStorageV1ContainerBasic,
				PlanOnly: true,
			},
		},
	})
}

// ---------------------------------------------------------------------------
// Unit test for the state-upgrade function (no acceptance infra required)
// ---------------------------------------------------------------------------

// TestObjectStorageV1ContainerUpgradeStateV0 exercises the UpgradeState
// entry for prior version 0 directly, without spinning up a real provider.
// This is the framework-idiomatic replacement for the SDKv2 unit test
// TestAccObjectStorageV1ContainerStateUpgradeV0.
func TestObjectStorageV1ContainerUpgradeStateV0(t *testing.T) {
	r := &objectStorageContainerV1Resource{}

	upgraders := r.UpgradeState(context.Background())

	upgrader, ok := upgraders[0]
	if !ok {
		t.Fatal("expected upgrader for version 0")
	}

	if upgrader.PriorSchema == nil {
		t.Fatal("upgrader for version 0 must have a non-nil PriorSchema")
	}

	// Verify that the PriorSchema contains the expected V0 attributes.
	if _, hasVersioning := upgrader.PriorSchema.Attributes["versioning"]; !hasVersioning {
		t.Error("PriorSchema for V0 must contain 'versioning' attribute (the old TypeSet field)")
	}

	if _, hasLegacy := upgrader.PriorSchema.Attributes["versioning_legacy"]; hasLegacy {
		t.Error("PriorSchema for V0 must NOT contain 'versioning_legacy' (that is a V1 field)")
	}

	if _, hasStorageClass := upgrader.PriorSchema.Attributes["storage_class"]; hasStorageClass {
		t.Error("PriorSchema for V0 must NOT contain 'storage_class' (that is a V1 field)")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func testAccCheckObjectStorageV1ContainerDestroy(ctx context.Context) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		// Retrieve provider config from the framework provider.
		// In a fully-migrated provider, access via the framework's provider
		// ConfigureResponse.  During migration, we fall back to testAccProvider
		// for the API client.
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
// Test config constants (unchanged from SDKv2 test file)
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
  versioning_legacy = [
    {
      type     = "versions"
      location = "othercontainer"
    }
  ]
  container_read  = ".r:*,.rlistings"
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
    test = "true"
    upperTest = "true"
  }
  content_type   = "application/json"
  storage_policy = "Policy-0"
}
`

// frameworkResourceForTesting is a helper that returns the resource type
// for unit-testing UpgradeState without needing a full provider server.
func frameworkResourceForTesting() resource.Resource {
	return NewObjectStorageContainerV1Resource()
}
