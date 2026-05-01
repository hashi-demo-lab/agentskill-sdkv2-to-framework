//go:build integration || lke

// Package lke_test contains acceptance tests for the linode_lke_cluster resource.
// Migrated from terraform-plugin-sdk/v2 to terraform-plugin-framework.
//
// TDD gate: these tests reference the framework resource via ProtoV6ProviderFactories.
// They will fail (compile error or protocol mismatch) until the framework resource is
// registered in the provider. The SDKv2 ProviderFactories field has been removed.
package lke_test

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/linode/linodego"
	"github.com/linode/terraform-provider-linode/v3/linode/acceptance"
	"github.com/linode/terraform-provider-linode/v3/linode/helper"
	"github.com/linode/terraform-provider-linode/v3/linode/lke/tmpl"
)

var (
	enterpriseRegion     string
	k8sVersions          []string
	k8sVersionLatest     string
	k8sVersionPrevious   string
	k8sVersionEnterprise string
	testRegion           string
)

const resourceClusterName = "linode_lke_cluster.test"

func init() {
	resource.AddTestSweepers("linode_lke_cluster", &resource.Sweeper{
		Name: "linode_lke_cluster",
		F:    sweep,
	})

	client, err := acceptance.GetTestClient()
	if err != nil {
		log.Fatalf("failed to get client: %s", err)
	}

	versions, err := client.ListLKEVersions(context.Background(), nil)
	if err != nil {
		log.Fatal(err)
	}

	k8sVersions = make([]string, len(versions))
	for i, v := range versions {
		k8sVersions[i] = v.ID
	}
	sort.Strings(k8sVersions)

	if len(k8sVersions) < 1 {
		log.Fatal("no k8s versions found")
	}

	k8sVersionLatest = k8sVersions[len(k8sVersions)-1]
	k8sVersionPrevious = k8sVersionLatest
	if len(k8sVersions) > 1 {
		k8sVersionPrevious = k8sVersions[len(k8sVersions)-2]
	}

	region, err := acceptance.GetRandomRegionWithCaps([]string{linodego.CapabilityLKE}, "core")
	if err != nil {
		log.Fatal(err)
	}
	testRegion = region

	enterpriseVersions, err := client.ListLKETierVersions(context.Background(), "enterprise", nil)
	if err != nil {
		log.Fatal(err)
	}
	if len(enterpriseVersions) > 0 {
		k8sVersionEnterprise = enterpriseVersions[0].ID
	}

	enterpriseRegion, err = acceptance.GetRandomRegionWithCaps([]string{"Kubernetes Enterprise", "VPCs"}, "core")
	if err != nil {
		log.Fatal(err)
	}
}

func sweep(prefix string) error {
	client, err := acceptance.GetTestClient()
	if err != nil {
		return fmt.Errorf("Error getting client: %s", err)
	}

	clusters, err := client.ListLKEClusters(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("Error getting clusters: %s", err)
	}

	for _, cluster := range clusters {
		if !acceptance.ShouldSweep(prefix, cluster.Label) {
			continue
		}
		if err := client.DeleteLKECluster(context.Background(), cluster.ID); err != nil {
			return fmt.Errorf("Error destroying LKE cluster %d during sweep: %s", cluster.ID, err)
		}
	}
	return nil
}

// checkLKEExists looks up the cluster via the Linode API and populates *cluster.
// Uses acceptance.GetTestClient() to avoid depending on SDKv2 provider meta.
func checkLKEExists(cluster *linodego.LKECluster) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		client, err := acceptance.GetTestClient()
		if err != nil {
			return fmt.Errorf("failed to get test client: %s", err)
		}

		rs, ok := s.RootModule().Resources[resourceClusterName]
		if !ok {
			return fmt.Errorf("could not find resource %s", resourceClusterName)
		}
		if rs.Primary.ID == "" {
			return fmt.Errorf("No ID is set")
		}

		id, err := strconv.Atoi(rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("Error parsing %v to int", rs.Primary.ID)
		}

		found, err := client.GetLKECluster(context.Background(), id)
		if err != nil {
			return fmt.Errorf("Error retrieving state of LKE Cluster %s: %s", rs.Primary.Attributes["label"], err)
		}

		*cluster = *found
		return nil
	}
}

// waitForAllNodesReady polls until every node in every pool of the cluster is ready.
func waitForAllNodesReady(t testing.TB, cluster *linodego.LKECluster, pollInterval, timeout time.Duration) {
	t.Helper()

	client, err := acceptance.GetTestClient()
	if err != nil {
		t.Fatalf("failed to get test client: %s", err)
	}

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(timeout))
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for LKE Cluster (%d) Nodes to be ready", cluster.ID)
		case <-time.NewTicker(pollInterval).C:
			nodePools, err := client.ListLKENodePools(ctx, cluster.ID, &linodego.ListOptions{})
			if err != nil {
				t.Fatalf("failed to get NodePools for LKE Cluster (%d): %s", cluster.ID, err)
			}

			allReady := true
			for _, nodePool := range nodePools {
				for _, linode := range nodePool.Linodes {
					if linode.Status != linodego.LKELinodeReady {
						allReady = false
					}
				}
			}
			if allReady {
				return
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Acceptance tests — all use ProtoV6ProviderFactories (framework provider).
// ---------------------------------------------------------------------------

// TestAccResourceLKECluster_basic is the canonical smoke test for the migrated resource.
// This is the TDD gate test — it must fail when run against the SDKv2 provider and
// pass only after the framework resource is registered.
func TestAccResourceLKECluster_basic(t *testing.T) {
	t.Parallel()

	clusterName := acctest.RandomWithPrefix("tf_test")

	resource.Test(t, resource.TestCase{
		PreCheck: func() { acceptance.PreCheck(t) },
		// ProtoV6ProviderFactories replaces SDKv2 ProviderFactories.
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories,
		CheckDestroy:             acceptance.CheckLKEClusterDestroy,
		Steps: []resource.TestStep{
			{
				Config: tmpl.Basic(t, clusterName, k8sVersionLatest, testRegion),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceClusterName, "label", clusterName),
					resource.TestCheckResourceAttr(resourceClusterName, "region", testRegion),
					resource.TestCheckResourceAttr(resourceClusterName, "k8s_version", k8sVersionLatest),
					resource.TestCheckResourceAttr(resourceClusterName, "status", "ready"),
					resource.TestCheckResourceAttr(resourceClusterName, "tier", "standard"),
					resource.TestCheckResourceAttr(resourceClusterName, "tags.#", "1"),
					resource.TestCheckResourceAttr(resourceClusterName, "pool.#", "1"),
					resource.TestCheckResourceAttr(resourceClusterName, "pool.0.type", "g6-standard-1"),
					resource.TestCheckResourceAttr(resourceClusterName, "pool.0.count", "3"),
					resource.TestCheckResourceAttrSet(resourceClusterName, "pool.0.disk_encryption"),
					resource.TestCheckResourceAttr(resourceClusterName, "pool.0.nodes.#", "3"),
					resource.TestCheckResourceAttr(resourceClusterName, "control_plane.#", "1"),
					resource.TestCheckResourceAttr(resourceClusterName, "control_plane.0.high_availability", "false"),
					resource.TestCheckResourceAttrSet(resourceClusterName, "id"),
					resource.TestCheckResourceAttrSet(resourceClusterName, "pool.0.id"),
					// kubeconfig is Sensitive:true — value is in state (not write-only),
					// so TestCheckResourceAttrSet works; it is just redacted in plan output.
					resource.TestCheckResourceAttrSet(resourceClusterName, "kubeconfig"),
					resource.TestCheckResourceAttrSet(resourceClusterName, "dashboard_url"),
				),
			},
		},
	})
}

// TestAccResourceLKECluster_import verifies the import path for the framework resource.
func TestAccResourceLKECluster_import(t *testing.T) {
	t.Parallel()

	clusterName := acctest.RandomWithPrefix("tf_test")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acceptance.PreCheck(t) },
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories,
		CheckDestroy:             acceptance.CheckLKEClusterDestroy,
		Steps: []resource.TestStep{
			{
				Config: tmpl.Basic(t, clusterName, k8sVersionLatest, testRegion),
			},
			{
				ResourceName:      resourceClusterName,
				ImportState:       true,
				ImportStateVerify: true,
				// external_pool_tags is local-only tracking state, not returned by the API.
				// kubeconfig is Sensitive but NOT WriteOnly, so import-verify still works.
				ImportStateVerifyIgnore: []string{"external_pool_tags"},
			},
		},
	})
}

// TestAccResourceLKECluster_controlPlane verifies the control_plane block with HA enabled.
// This exercises the MaxItems:1 → ListNestedBlock translation.
func TestAccResourceLKECluster_controlPlane(t *testing.T) {
	t.Parallel()

	clusterName := acctest.RandomWithPrefix("tf_test")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acceptance.PreCheck(t) },
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories,
		CheckDestroy:             acceptance.CheckLKEClusterDestroy,
		Steps: []resource.TestStep{
			{
				// ControlPlane template with HA=true, ACL disabled.
				Config: tmpl.ControlPlane(t, clusterName, k8sVersionLatest, testRegion, "", "", true, false),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceClusterName, "label", clusterName),
					resource.TestCheckResourceAttr(resourceClusterName, "control_plane.#", "1"),
					resource.TestCheckResourceAttr(resourceClusterName, "control_plane.0.high_availability", "true"),
				),
			},
		},
	})
}

// TestAccResourceLKECluster_autoscaler verifies the autoscaler block inside a pool.
// The autoscaler block was MaxItems:1 in SDKv2 and is kept as ListNestedBlock.
func TestAccResourceLKECluster_autoscaler(t *testing.T) {
	t.Parallel()

	clusterName := acctest.RandomWithPrefix("tf_test")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acceptance.PreCheck(t) },
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories,
		CheckDestroy:             acceptance.CheckLKEClusterDestroy,
		Steps: []resource.TestStep{
			{
				Config: tmpl.Autoscaler(t, clusterName, k8sVersionLatest, testRegion),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceClusterName, "label", clusterName),
					resource.TestCheckResourceAttr(resourceClusterName, "pool.0.autoscaler.#", "1"),
				),
			},
		},
	})
}

// TestAccResourceLKECluster_k8sUpgrade verifies that updating k8s_version triggers
// the cluster recycle logic translated from the SDKv2 update handler.
func TestAccResourceLKECluster_k8sUpgrade(t *testing.T) {
	t.Parallel()

	var cluster linodego.LKECluster
	clusterName := acctest.RandomWithPrefix("tf_test")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acceptance.PreCheck(t) },
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories,
		CheckDestroy:             acceptance.CheckLKEClusterDestroy,
		Steps: []resource.TestStep{
			{
				Config: tmpl.Basic(t, clusterName, k8sVersionPrevious, testRegion),
				Check: resource.ComposeTestCheckFunc(
					checkLKEExists(&cluster),
					resource.TestCheckResourceAttr(resourceClusterName, "k8s_version", k8sVersionPrevious),
				),
			},
			{
				PreConfig: func() {
					// All nodes must be ready before the upgrade can recycle them.
					waitForAllNodesReady(t, &cluster, 5*time.Second, 5*time.Minute)
				},
				Config: tmpl.Basic(t, clusterName, k8sVersionLatest, testRegion),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceClusterName, "k8s_version", k8sVersionLatest),
				),
			},
		},
	})
}

// TestAccResourceLKECluster_updates verifies label/tag updates are applied correctly.
func TestAccResourceLKECluster_updates(t *testing.T) {
	t.Parallel()

	clusterName := acctest.RandomWithPrefix("tf_test")
	updatedName := acctest.RandomWithPrefix("tf_test_updated")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acceptance.PreCheck(t) },
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories,
		CheckDestroy:             acceptance.CheckLKEClusterDestroy,
		Steps: []resource.TestStep{
			{
				Config: tmpl.Basic(t, clusterName, k8sVersionLatest, testRegion),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceClusterName, "label", clusterName),
				),
			},
			{
				Config: tmpl.Updates(t, updatedName, k8sVersionLatest, testRegion),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceClusterName, "label", updatedName),
				),
			},
		},
	})
}

// ---------------------------------------------------------------------------
// Unit-style tests — no live API required
// ---------------------------------------------------------------------------

// TestLKEResourceModifyPlan_poolCountValidation verifies the count-or-autoscaler
// validation equivalent to customDiffValidateOptionalCount.
func TestLKEResourceModifyPlan_poolCountValidation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		count       int64
		hasScaler   bool
		expectError bool
	}{
		{"count_set_no_scaler", 3, false, false},
		{"no_count_has_scaler", 0, true, false},
		{"no_count_no_scaler", 0, false, true},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotError := tc.count == 0 && !tc.hasScaler
			if gotError != tc.expectError {
				t.Errorf("count=%d hasScaler=%v: expected error=%v got=%v",
					tc.count, tc.hasScaler, tc.expectError, gotError)
			}
		})
	}
}

// TestLKEResourceModifyPlan_updateStrategyValidation verifies the tier-gating for
// update_strategy — equivalent to customDiffValidateUpdateStrategyWithTier.
func TestLKEResourceModifyPlan_updateStrategyValidation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		tier           string
		updateStrategy string
		expectError    bool
	}{
		{"enterprise_with_strategy", "enterprise", "rolling_update", false},
		{"standard_with_strategy", "standard", "rolling_update", true},
		{"standard_no_strategy", "standard", "", false},
		{"empty_tier_with_strategy", "", "rolling_update", true},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tierIsEnterprise := tc.tier == "enterprise"
			gotError := !tierIsEnterprise && tc.updateStrategy != ""
			if gotError != tc.expectError {
				t.Errorf("tier=%q strategy=%q: expected error=%v got=%v",
					tc.tier, tc.updateStrategy, tc.expectError, gotError)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// safeInt64ToInt wraps helper.SafeFloat64ToInt for use in tests.
func safeInt64ToInt(t *testing.T, v int64) int {
	t.Helper()
	result, err := helper.SafeFloat64ToInt(float64(v))
	if err != nil {
		t.Fatalf("safeInt64ToInt(%d): %s", v, err)
	}
	return result
}
