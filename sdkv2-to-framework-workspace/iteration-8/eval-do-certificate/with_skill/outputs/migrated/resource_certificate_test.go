package certificate_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/digitalocean/godo"
	"github.com/digitalocean/terraform-provider-digitalocean/digitalocean/acceptance"
	"github.com/digitalocean/terraform-provider-digitalocean/digitalocean/certificate"
	"github.com/digitalocean/terraform-provider-digitalocean/digitalocean/config"
	"github.com/digitalocean/terraform-provider-digitalocean/digitalocean/util"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// protoV6ProviderFactories serves the provider via the framework protocol (v6).
// Replace NewDigitalOceanFrameworkProvider with the actual constructor once the
// provider's main registration layer has been migrated from SDKv2 to framework.
var protoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"digitalocean": providerserver.NewProtocol6WithError(
		acceptance.NewFrameworkProvider(),
	),
}

// godoClientFromState extracts the godo.Client from the acceptance test
// provider meta. This helper uses TestAccProvider which is still SDKv2-backed
// during the transition; replace with the framework meta-accessor once the
// provider's Configure method is on the framework path.
func godoClientFromState() *godo.Client {
	return acceptance.TestAccProvider.Meta().(*config.CombinedConfig).GodoClient()
}

// --- State-upgrader unit test ---

// TestResourceCertificateStateUpgradeV0 exercises the exported
// MigrateCertificateStateV0toV1 shim, which encodes the same logic as the
// framework UpgradeState(0) upgrader. Keeping it as a pure raw-map function
// allows unit-testing the migration without a live Terraform run.
func TestResourceCertificateStateUpgradeV0(t *testing.T) {
	ctx := context.Background()

	rawV0 := map[string]interface{}{
		"id":   "aaa-bbb-123-ccc",
		"name": "test",
	}

	// V1 swaps id ↔ name and adds uuid (the former id).
	expectedV1 := map[string]interface{}{
		"id":   "test",
		"uuid": "aaa-bbb-123-ccc",
		"name": "test",
	}

	actual, err := certificate.MigrateCertificateStateV0toV1(ctx, rawV0, nil)
	if err != nil {
		t.Fatalf("error migrating state: %s", err)
	}

	for k, want := range expectedV1 {
		got := actual[k]
		if got != want {
			t.Errorf("key %q: expected %v, got %v", k, want, got)
		}
	}
}

// --- Acceptance tests ---

func TestAccDigitalOceanCertificate_Basic(t *testing.T) {
	var cert godo.Certificate
	name := acceptance.RandomTestName("certificate")
	privateKeyMaterial, leafCertMaterial, certChainMaterial := acceptance.GenerateTestCertMaterial(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acceptance.TestAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckDigitalOceanCertificateDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccCheckDigitalOceanCertificateConfig_basic(name, privateKeyMaterial, leafCertMaterial, certChainMaterial),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckDigitalOceanCertificateExists("digitalocean_certificate.foobar", &cert),
					resource.TestCheckResourceAttr(
						"digitalocean_certificate.foobar", "id", name),
					resource.TestCheckResourceAttr(
						"digitalocean_certificate.foobar", "name", name),
					// private_key, leaf_certificate, and certificate_chain are stored
					// as SHA-1 hashes in state (hash-in-CRUD pattern, replacing the
					// SDKv2 StateFunc approach — per the skill, ValueFromString MUST NOT
					// hash; hashing happens in Create/Read instead).
					resource.TestCheckResourceAttr(
						"digitalocean_certificate.foobar", "private_key",
						util.HashString(fmt.Sprintf("%s\n", privateKeyMaterial))),
					resource.TestCheckResourceAttr(
						"digitalocean_certificate.foobar", "leaf_certificate",
						util.HashString(fmt.Sprintf("%s\n", leafCertMaterial))),
					resource.TestCheckResourceAttr(
						"digitalocean_certificate.foobar", "certificate_chain",
						util.HashString(fmt.Sprintf("%s\n", certChainMaterial))),
				),
			},
			{
				// Import by certificate name (the stable Terraform resource ID).
				ResourceName:      "digitalocean_certificate.foobar",
				ImportState:       true,
				ImportStateVerify: true,
				// The DigitalOcean API does not return PEM material; imported state
				// will have null for these fields. Ignoring prevents a false failure.
				ImportStateVerifyIgnore: []string{
					"private_key",
					"leaf_certificate",
					"certificate_chain",
				},
			},
		},
	})
}

func TestAccDigitalOceanCertificate_ExpectedErrors(t *testing.T) {
	name := acceptance.RandomTestName("certificate")
	privateKeyMaterial, leafCertMaterial, certChainMaterial := acceptance.GenerateTestCertMaterial(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acceptance.TestAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		CheckDestroy:             testAccCheckDigitalOceanCertificateDestroy,
		Steps: []resource.TestStep{
			{
				Config:      testAccCheckDigitalOceanCertificateConfig_customNoLeaf(name, privateKeyMaterial, certChainMaterial),
				ExpectError: regexp.MustCompile("`leaf_certificate` is required for when type is `custom` or empty"),
			},
			{
				Config:      testAccCheckDigitalOceanCertificateConfig_customNoKey(name, leafCertMaterial, certChainMaterial),
				ExpectError: regexp.MustCompile("`private_key` is required for when type is `custom` or empty"),
			},
			{
				Config:      testAccCheckDigitalOceanCertificateConfig_noDomains(name),
				ExpectError: regexp.MustCompile("`domains` is required for when type is `lets_encrypt`"),
			},
		},
	})
}

// --- Helper check functions ---

func testAccCheckDigitalOceanCertificateDestroy(s *terraform.State) error {
	client := godoClientFromState()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "digitalocean_certificate" {
			continue
		}
		_, err := certificate.FindCertificateByName(client, rs.Primary.ID)
		if err != nil && !strings.Contains(err.Error(), "not found") {
			return fmt.Errorf(
				"error waiting for certificate (%s) to be destroyed: %s",
				rs.Primary.ID, err)
		}
	}
	return nil
}

func testAccCheckDigitalOceanCertificateExists(n string, cert *godo.Certificate) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s", n)
		}
		if rs.Primary.ID == "" {
			return fmt.Errorf("no Certificate ID is set")
		}
		client := godoClientFromState()
		c, err := certificate.FindCertificateByName(client, rs.Primary.ID)
		if err != nil {
			return err
		}
		*cert = *c
		return nil
	}
}

// --- Config templates ---

func testAccCheckDigitalOceanCertificateConfig_basic(name, privateKeyMaterial, leafCert, certChain string) string {
	return fmt.Sprintf(`
resource "digitalocean_certificate" "foobar" {
  name              = "%s"
  private_key       = <<EOF
%s
EOF
  leaf_certificate  = <<EOF
%s
EOF
  certificate_chain = <<EOF
%s
EOF
}`, name, privateKeyMaterial, leafCert, certChain)
}

func testAccCheckDigitalOceanCertificateConfig_customNoLeaf(name, privateKeyMaterial, certChain string) string {
	return fmt.Sprintf(`
resource "digitalocean_certificate" "foobar" {
  name              = "%s"
  private_key       = <<EOF
%s
EOF
  certificate_chain = <<EOF
%s
EOF
}`, name, privateKeyMaterial, certChain)
}

func testAccCheckDigitalOceanCertificateConfig_customNoKey(name, leafCert, certChain string) string {
	return fmt.Sprintf(`
resource "digitalocean_certificate" "foobar" {
  name              = "%s"
  leaf_certificate  = <<EOF
%s
EOF
  certificate_chain = <<EOF
%s
EOF
}`, name, leafCert, certChain)
}

func testAccCheckDigitalOceanCertificateConfig_noDomains(name string) string {
	return fmt.Sprintf(`
resource "digitalocean_certificate" "foobar" {
  name = "%s"
  type = "lets_encrypt"
}`, name)
}
