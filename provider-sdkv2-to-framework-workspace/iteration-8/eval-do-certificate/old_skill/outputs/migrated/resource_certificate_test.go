package certificate_test

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/digitalocean/godo"
	"github.com/digitalocean/terraform-provider-digitalocean/digitalocean/acceptance"
	"github.com/digitalocean/terraform-provider-digitalocean/digitalocean/certificate"
	"github.com/digitalocean/terraform-provider-digitalocean/digitalocean/util"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"golang.org/x/oauth2"
)

// protoV6ProviderFactories constructs the framework provider for acceptance tests.
// Replace `digitalocean.NewFrameworkProvider()` with however the top-level
// provider constructor is named once the full provider migration is complete.
var protoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"digitalocean": providerserver.NewProtocol6WithError(acceptance.NewFrameworkProvider()),
}

// testGodoClient creates a raw godo.Client from the DIGITALOCEAN_TOKEN environment
// variable for use in destroy/exists check functions (where provider meta is not
// available through the framework's protocol boundary).
func testGodoClient(t *testing.T) *godo.Client {
	t.Helper()
	token := os.Getenv("DIGITALOCEAN_TOKEN")
	if token == "" {
		t.Skip("DIGITALOCEAN_TOKEN must be set for acceptance tests")
	}
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	oauthClient := oauth2.NewClient(context.Background(), ts)
	return godo.NewClient(oauthClient)
}

// ---------------------------------------------------------------------------
// State-upgrade unit test — no TF_ACC required
// ---------------------------------------------------------------------------

func testCertificateStateDataV0() map[string]interface{} {
	return map[string]interface{}{
		"name": "test",
		"id":   "aaa-bbb-123-ccc",
	}
}

func testCertificateStateDataV1() map[string]interface{} {
	v0 := testCertificateStateDataV0()
	return map[string]interface{}{
		"name": v0["name"],
		"uuid": v0["id"],
		"id":   v0["name"],
	}
}

func TestResourceExampleInstanceStateUpgradeV0(t *testing.T) {
	expected := testCertificateStateDataV1()
	actual, err := certificate.MigrateCertificateStateV0toV1(context.Background(), testCertificateStateDataV0(), nil)
	if err != nil {
		t.Fatalf("error migrating state: %s", err)
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("\n\nexpected:\n\n%#v\n\ngot:\n\n%#v\n\n", actual, expected)
	}
}

// ---------------------------------------------------------------------------
// Acceptance tests
// ---------------------------------------------------------------------------

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
					resource.TestCheckResourceAttr(
						"digitalocean_certificate.foobar", "private_key", util.HashString(fmt.Sprintf("%s\n", privateKeyMaterial))),
					resource.TestCheckResourceAttr(
						"digitalocean_certificate.foobar", "leaf_certificate", util.HashString(fmt.Sprintf("%s\n", leafCertMaterial))),
					resource.TestCheckResourceAttr(
						"digitalocean_certificate.foobar", "certificate_chain", util.HashString(fmt.Sprintf("%s\n", certChainMaterial))),
				),
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

// ---------------------------------------------------------------------------
// Helper check functions
// ---------------------------------------------------------------------------

func testAccCheckDigitalOceanCertificateDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "digitalocean_certificate" {
			continue
		}

		// During framework migration the provider meta is no longer accessible
		// through the SDKv2 TestAccProvider — create a fresh godo client.
		client := godo.NewClient(oauth2.NewClient(
			context.Background(),
			oauth2.StaticTokenSource(&oauth2.Token{AccessToken: os.Getenv("DIGITALOCEAN_TOKEN")}),
		))

		_, err := certificate.FindCertificateByName(client, rs.Primary.ID)

		if err != nil && !strings.Contains(err.Error(), "not found") {
			return fmt.Errorf(
				"Error waiting for certificate (%s) to be destroyed: %s",
				rs.Primary.ID, err)
		}
	}

	return nil
}

func testAccCheckDigitalOceanCertificateExists(n string, cert *godo.Certificate) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("No Certificate ID is set")
		}

		client := godo.NewClient(oauth2.NewClient(
			context.Background(),
			oauth2.StaticTokenSource(&oauth2.Token{AccessToken: os.Getenv("DIGITALOCEAN_TOKEN")}),
		))

		c, err := certificate.FindCertificateByName(client, rs.Primary.ID)
		if err != nil {
			return err
		}

		*cert = *c

		return nil
	}
}

// ---------------------------------------------------------------------------
// Config helpers
// ---------------------------------------------------------------------------

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
