package tlsclouddatastore_test

import (
	"net/url"
	"testing"

	"reflect"

	"context"

	"os"

	"cloud.google.com/go/datastore"
	"github.com/hashicorp/consul/api"
	"github.com/j0hnsmith/caddy-tlsclouddatastore"
	"github.com/mholt/caddy/caddytls"
	"google.golang.org/api/iterator"
)

var consulClient *api.Client

const TestCaUrl = "https://acme-staging.api.letsencrypt.org/directory"

// these tests need a Cloud Datastore emulator `gcloud beta emulators datastore start`
// https://cloud.google.com/datastore/docs/tools/datastore-emulator
func setupStorage(t *testing.T) caddytls.Storage {
	truncateDs(t)

	caurl, _ := url.Parse(TestCaUrl)
	cs, err := tlsclouddatastore.NewCloudDatastoreStorage(caurl)

	if err != nil {
		t.Fatalf("Error creating Consul storage: %v", err)
	}

	return cs
}

func truncateDs(t *testing.T) {
	projectID := os.Getenv(tlsclouddatastore.EnvNameProjectId)
	if projectID == "" {
		t.Fatalf("Unable read project id from env var: %s", tlsclouddatastore.EnvNameProjectId)
	}

	cloudDsClient, err := datastore.NewClient(context.TODO(), projectID)
	if err != nil {
		t.Fatalf("Unable to create Cloud Datastore client: %v", err)
	}

	recordTypes := []string{tlsclouddatastore.USER_RECORD, tlsclouddatastore.SITE_RECORD}
	for _, rt := range recordTypes {
		q := datastore.NewQuery(rt).KeysOnly()
		for it := cloudDsClient.Run(context.TODO(), q); ; {
			key, err := it.Next(nil)
			if err == iterator.Done {
				break
			}
			if err != nil {
				t.Fatal(err)
			}

			if err := cloudDsClient.Delete(context.TODO(), key); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func getUser() *caddytls.UserData {
	return &caddytls.UserData{
		Reg: []byte("registration"),
		Key: []byte("key"),
	}
}

func getSite() *caddytls.SiteData {
	return &caddytls.SiteData{
		Cert: []byte("cert"),
		Key:  []byte("key"),
		Meta: []byte("meta"),
	}
}

func TestMostRecentUserEmail(t *testing.T) {
	gds := setupStorage(t)

	email := gds.MostRecentUserEmail()
	if email != "" {
		t.Fatalf("email should be empty if nothing found")
	}

	gds.StoreUser("test@test.com", getUser())
	email = gds.MostRecentUserEmail()
	if email != "test@test.com" {
		t.Fatalf("'%s' doesn't match 'test@test.com'", email)
	}

	gds.StoreUser("test2@test.com", getUser())
	email = gds.MostRecentUserEmail()
	if email != "test2@test.com" {
		t.Fatalf("email should be the newest user but found %s", email)
	}

}

func TestStoreAndLoadUser(t *testing.T) {
	gds := setupStorage(t)

	defaultUser := getUser()
	err := gds.StoreUser("test@test.com", defaultUser)
	if err != nil {
		t.Fatalf("Error storing user: %v", err)
	}

	user, err := gds.LoadUser("test@test.com")
	if err != nil {
		t.Fatalf("Error loading user: %v", err)
	}
	if !reflect.DeepEqual(user, defaultUser) {
		t.Fatalf("Loaded user is not the same like the saved one")
	}
}

func TestStoreAndLoadSite(t *testing.T) {
	gds := setupStorage(t)

	defaultSite := getSite()

	err := gds.StoreSite("tls.test.com", defaultSite)
	if err != nil {
		t.Fatalf("Error storing site: %v", err)
	}

	site, err := gds.LoadSite("tls.test.com")
	if err != nil {
		t.Fatalf("Error loading site: %v", err)
	}
	if !reflect.DeepEqual(site, defaultSite) {
		t.Fatalf("Loaded site is not the same like the saved one")
	}

	err = gds.DeleteSite("tls.test.com")
	if err != nil {
		t.Fatalf("Error deleting site: %v", err)
	}

	site, err = gds.LoadSite("tls.test.com")
	if site != nil {
		t.Fatal("Site should be deleted")
	}
}
