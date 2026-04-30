package certificate_test

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/digitalocean/godo"
	"github.com/digitalocean/terraform-provider-digitalocean/digitalocean/acceptance"
	"github.com/digitalocean/terraform-provider-digitalocean/digitalocean/certificate"
	"github.com/digitalocean/terraform-provider-digitalocean/digitalocean/config"
	"github.com/digitalocean/terraform-provider-digitalocean/digitalocean/util"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// testCertificateStateDataV0 / V1 + TestResourceExampleInstanceStateUpgradeV0
// continue to exercise the *raw map* migration helper — the framework
// `UpgradeState` method is a thin wrapper around the same transformation, so
// keeping the existing unit test intact gives us coverage of the migration
// logic without depending on the framework's runtime upgrade machinery.

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

// Acceptance tests run via the muxed protocol-v6 factory so the SDKv2 helpers
// (resource.Test*, terraform.State, etc.) continue to work — they accept any
// provider that speaks the Terraform plugin protocol.
//
// Once the certificate resource is migrated to the framework, the surrounding
// provider must serve it through `protoV6ProviderFactories` (defined in the
// acceptance package). For the purposes of this migration, we use whatever
// the acceptance package exposes; the existing `TestAccProviderFactories`
// continues to work for tests that touch the SDKv2 server, and the migrated
// resource is expected to be wired into the framework provider that's served
// alongside it.

func TestAccDigitalOceanCertificate_Basic(t *testing.T) {
	var cert godo.Certificate
	name := acceptance.RandomTestName("certificate")
	privateKeyMaterial, leafCertMaterial, certChainMaterial := acceptance.GenerateTestCertMaterial(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acceptance.TestAccPreCheck(t) },
		ProtoV6ProviderFactories: acceptance.TestAccProtoV6ProviderFactories,
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
		ProtoV6ProviderFactories: acceptance.TestAccProtoV6ProviderFactories,
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

func testAccCheckDigitalOceanCertificateDestroy(s *terraform.State) error {
	client := acceptance.TestAccProvider.Meta().(*config.CombinedConfig).GodoClient()

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "digitalocean_certificate" {
			continue
		}

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

		client := acceptance.TestAccProvider.Meta().(*config.CombinedConfig).GodoClient()

		c, err := certificate.FindCertificateByName(client, rs.Primary.ID)
		if err != nil {
			return err
		}

		*cert = *c

		return nil
	}
}

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
