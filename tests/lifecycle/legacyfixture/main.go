// Built against the pinned pre-auto-delete source by the legacy state tests.
// Only the HTTP transport is replaced; the old provider writes its own state.
package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/shadeform/terraform-provider-shadeform/internal/provider"
	"github.com/shadeform/terraform-provider-shadeform/tests/lifecycle/internal/mockapi"
)

func main() {
	file := os.Getenv("SHADEFORM_LIFECYCLE_STATE")
	if file == "" {
		log.Fatal("SHADEFORM_LIFECYCLE_STATE is required; this executable cannot use the live API")
	}
	// The legacy client uses http.Client with a nil Transport. Replacing the
	// process default before serving prevents all HTTP from reaching the network.
	http.DefaultTransport = mockapi.Transport{File: file}
	if err := providerserver.Serve(context.Background(), provider.New("legacy-state-test"), providerserver.ServeOpts{
		Address: "registry.terraform.io/shadeform/shadeform",
	}); err != nil {
		log.Fatal(err)
	}
}
